package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, works with CGO_ENABLED=0 on all targets
	"net.basov.omngo/backend/internal/logx"
)

// ----------------------------------------------------------------------
// SQLite on the server, for note scripts
// ----------------------------------------------------------------------
//
// The browsers removed WebSQL (window.openDatabase), and it kept the data of
// a note in each browser apart. A database is thus an SQLite file under
// <StorageDir>/db/<name>.sqlite, and each device sees the same data.
//
// POST /api/sql takes and answers JSON:
//
//	request:  { "db": "mydata",
//	            "statements": [ {"sql": "INSERT ... VALUES(?,?)", "args": [1, "x"]},
//	                            {"sql": "SELECT * FROM t", "args": []} ] }
//	response: { "status": "success",
//	            "results": [ {"rows_affected":1, "last_insert_id":7},
//	                         {"columns":["a","b"], "rows":[[1,"x"]],
//	                          "rows_affected":0, "last_insert_id":0} ] }
//
// EACH statement of one request runs in ONE transaction. A failure rolls the
// whole batch back, and the answer names the statement. The transaction() of
// the JS shim gets its atomicity from this.
//
// The limits are on purpose:
//
//	- Admin only. SQL is arbitrary code over shared state. A connection from
//	  the device itself passes, the same as elsewhere.
//	- A database name matches [A-Za-z0-9_-] with at most 64 characters. The
//	  name is a file name, thus this is the path-traversal guard.
//	- At most 1 MB of body and 500 statements for each request.
//
// The backup*.go files hold the JSONL backup and restore of these
// databases.

// dbNameRe allows only safe database names, because a name is a file name.
var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

const (
	sqlMaxBodyBytes = 1 << 20 // 1 MB
	MaxStatements   = 500
)

// Open answers the handle of a named database, and it opens or makes
// the file at the first use. Then, without the lock, it runs the one
// automatic restore. That restore is for a database with backups and no
// .sqlite file, as on a fresh device after a pull. Each other restore is
// manual. See backup.go.
func (svc Service) Open(name string) (*sql.DB, error) {
	db, err := svc.openLocked(name)
	if err != nil {
		return nil, err
	}
	if reopened, err := svc.bootstrapIfMissing(name); err != nil {
		// A failed bootstrap must not stop the database. The note script then
		// sees an empty database. The backup file stays for a manual restore.
		svc.Log(logx.DBBootstrap).Errf("%s: %v", name, err)
	} else if reopened != nil {
		// The bootstrap replaced the file and evicted the handle above. Give
		// out the new handle.
		return reopened, nil
	}
	return db, nil
}

// openLocked opens a database, or answers the cached handle, under
// svc.Store.mu. Open never holds the lock while bootstrapIfMissing runs a
// whole restore.
func (svc Service) openLocked(name string) (*sql.DB, error) {
	if !dbNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid database name %q (allowed: letters, digits, '_', '-', max 64 chars)", name)
	}

	svc.Store.mu.Lock()
	defer svc.Store.mu.Unlock()

	if svc.Store.dbs == nil {
		svc.Store.dbs = make(map[string]*sql.DB)
	}
	if db, ok := svc.Store.dbs[name]; ok {
		return db, nil
	}

	dir := svc.Layout.DB()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// busy_timeout lets a second request wait. journal_mode is TRUNCATE and
	// not WAL on purpose. WAL needs a memory-mapped -shm file, and that file
	// is not reliable on the FUSE storage of Android. TRUNCATE uses plain
	// file I/O.
	dsn := "file:" + filepath.Join(dir, name+".sqlite") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(TRUNCATE)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// Use one connection for each database. Requests then wait in a queue,
	// and they do not get SQLITE_BUSY.
	db.SetMaxOpenConns(1)

	svc.Store.dbs[name] = db
	return db, nil
}

type Statement struct {
	SQL  string        `json:"sql"`
	Args []interface{} `json:"args"`
}

type Request struct {
	DB         string      `json:"db"`
	Statements []Statement `json:"statements"`
}

type Result struct {
	Columns      []string        `json:"columns,omitempty"`
	Rows         [][]interface{} `json:"rows,omitempty"`
	RowsAffected int64           `json:"rows_affected"`
	LastInsertID int64           `json:"last_insert_id"`
}

type Response struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	// FailedStatement is the index of the statement that failed. The handler
	// sets it only when one statement caused the error.
	FailedStatement *int     `json:"failed_statement,omitempty"`
	Results         []Result `json:"results,omitempty"`
}

// returnsRows chooses Query or Exec from the first keyword. "WITH ... INSERT"
// counts as a query and loses its rows_affected. The fault is on the safe
// side: an empty result or a zero count, never damaged data.
func returnsRows(query string) bool {
	q := strings.ToUpper(strings.TrimSpace(query))
	for _, kw := range []string{"SELECT", "WITH", "PRAGMA", "EXPLAIN", "VALUES"} {
		if strings.HasPrefix(q, kw) {
			return true
		}
	}
	return false
}

// Evict closes and forgets a cached handle, thus the next Open
// opens the file again. With isStaleDBHandleError, it lets /api/sql recover
// from an error of the SQLITE_READONLY_DBMOVED class. Without the pair, each
// query fails until a restart. See
// doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md for the
// known cause.
func (svc Service) Evict(name string) {
	svc.Store.mu.Lock()
	db, ok := svc.Store.dbs[name]
	if ok {
		delete(svc.Store.dbs, name)
	}
	svc.Store.mu.Unlock()
	if ok {
		if err := db.Close(); err != nil {
			svc.Log(logx.DB).Errf("close evicted handle for %q: %v", name, err)
		}
	}
}

// isStaleDBHandleError tells whether err is SQLITE_READONLY_DBMOVED, code
// 1032. SQLite raises it when another writer replaced the file below an open
// connection. The driver checks the path at each write. The test reads the
// message text, "attempt to write a readonly database (1032)", and needs no
// type of the driver.
func isStaleDBHandleError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "readonly database") ||
		strings.Contains(msg, "SQLITE_READONLY") ||
		strings.Contains(msg, "(1032)")
}

// RunBatch runs the statements against dbName as one transaction.
// After a stale-handle error, it evicts the handle, opens the database again,
// and runs the batch ONE more time. A retry is safe, because the first
// attempt committed nothing.
func (svc Service) RunBatch(dbName string, statements []Statement) ([]Result, *int, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		db, err := svc.Open(dbName)
		if err != nil {
			return nil, nil, err
		}

		tx, err := db.Begin()
		if err != nil {
			if attempt == 1 && isStaleDBHandleError(err) {
				svc.Log(logx.DB).Infof("%s: stale handle on begin, reopening and retrying: %v", dbName, err)
				svc.Evict(dbName)
				lastErr = err
				continue
			}
			return nil, nil, fmt.Errorf("begin: %w", err)
		}

		results := make([]Result, 0, len(statements))
		var failedIdx *int
		var stmtErr error
		for i, stmt := range statements {
			var res Result
			res, stmtErr = runStatement(tx, stmt)
			if stmtErr != nil {
				idx := i
				failedIdx = &idx
				break
			}
			results = append(results, res)
		}

		if stmtErr != nil {
			tx.Rollback()
			if attempt == 1 && isStaleDBHandleError(stmtErr) {
				svc.Log(logx.DB).Infof("%s: stale handle on statement #%d, reopening and retrying: %v", dbName, *failedIdx, stmtErr)
				svc.Evict(dbName)
				lastErr = stmtErr
				continue
			}
			return nil, failedIdx, stmtErr
		}

		if err := tx.Commit(); err != nil {
			if attempt == 1 && isStaleDBHandleError(err) {
				svc.Log(logx.DB).Infof("%s: stale handle on commit, reopening and retrying: %v", dbName, err)
				svc.Evict(dbName)
				lastErr = err
				continue
			}
			return nil, nil, fmt.Errorf("commit: %w", err)
		}

		return results, nil, nil
	}
	return nil, nil, fmt.Errorf("after retry: %w", lastErr)
}

// HandleSQL runs one atomic batch against one named database. See the banner
// for the protocol.
func (svc Service) HandleSQL(w http.ResponseWriter, r *http.Request) {

	var req Request
	r.Body = http.MaxBytesReader(w, r.Body, sqlMaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		svc.writeJSON(w, http.StatusBadRequest, Response{Status: "error", Message: "bad request: " + err.Error()})
		return
	}
	if len(req.Statements) == 0 {
		svc.writeJSON(w, http.StatusBadRequest, Response{Status: "error", Message: "no statements"})
		return
	}
	if len(req.Statements) > MaxStatements {
		svc.writeJSON(w, http.StatusBadRequest, Response{Status: "error",
			Message: fmt.Sprintf("too many statements (%d > %d)", len(req.Statements), MaxStatements)})
		return
	}

	results, failedIdx, err := svc.RunBatch(req.DB, req.Statements)
	if err != nil {
		svc.writeJSON(w, http.StatusBadRequest, Response{
			Status:          "error",
			Message:         err.Error(),
			FailedStatement: failedIdx,
		})
		return
	}
	svc.writeJSON(w, http.StatusOK, Response{Status: "success", Results: results})
}

func runStatement(tx *sql.Tx, stmt Statement) (Result, error) {
	if !returnsRows(stmt.SQL) {
		res, err := tx.Exec(stmt.SQL, stmt.Args...)
		if err != nil {
			return Result{}, err
		}
		affected, _ := res.RowsAffected()
		lastID, _ := res.LastInsertId()
		return Result{RowsAffected: affected, LastInsertID: lastID}, nil
	}

	rows, err := tx.Query(stmt.SQL, stmt.Args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}
	out := Result{Columns: cols, Rows: [][]interface{}{}}

	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Result{}, err
		}
		// A []byte would encode as base64 in JSON. Send text as text.
		for i, v := range raw {
			if b, ok := v.([]byte); ok {
				raw[i] = string(b)
			}
		}
		out.Rows = append(out.Rows, raw)
	}
	return out, rows.Err()
}
