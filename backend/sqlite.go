package backend

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
// The db_backup*.go files hold the JSONL backup and restore of these
// databases.

// dbNameRe allows only safe database names, because a name is a file name.
var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

const (
	sqlMaxBodyBytes  = 1 << 20 // 1 MB
	sqlMaxStatements = 500
)

// openUserDB answers the handle of a named database, and it opens or makes
// the file at the first use. Then, without the lock, it runs the one
// automatic restore. That restore is for a database with backups and no
// .sqlite file, as on a fresh device after a pull. Each other restore is
// manual. See db_backup.go.
func (a *App) openUserDB(name string) (*sql.DB, error) {
	db, err := a.openUserDBLocked(name)
	if err != nil {
		return nil, err
	}
	if reopened, err := a.bootstrapIfMissing(name); err != nil {
		// A failed bootstrap must not stop the database. The note script then
		// sees an empty database. The backup file stays for a manual restore.
		a.log(logx.DBBootstrap).Errf("%s: %v", name, err)
	} else if reopened != nil {
		// The bootstrap replaced the file and evicted the handle above. Give
		// out the new handle.
		return reopened, nil
	}
	return db, nil
}

// openUserDBLocked opens a database, or answers the cached handle, under
// a.sqlMu. openUserDB never holds the lock while bootstrapIfMissing runs a
// whole restore.
func (a *App) openUserDBLocked(name string) (*sql.DB, error) {
	if !dbNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid database name %q (allowed: letters, digits, '_', '-', max 64 chars)", name)
	}

	a.sqlMu.Lock()
	defer a.sqlMu.Unlock()

	if a.sqlDBs == nil {
		a.sqlDBs = make(map[string]*sql.DB)
	}
	if db, ok := a.sqlDBs[name]; ok {
		return db, nil
	}

	dir := a.layout().db()
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

	a.sqlDBs[name] = db
	return db, nil
}

type sqlStatement struct {
	SQL  string        `json:"sql"`
	Args []interface{} `json:"args"`
}

type sqlRequest struct {
	DB         string         `json:"db"`
	Statements []sqlStatement `json:"statements"`
}

type sqlResult struct {
	Columns      []string        `json:"columns,omitempty"`
	Rows         [][]interface{} `json:"rows,omitempty"`
	RowsAffected int64           `json:"rows_affected"`
	LastInsertID int64           `json:"last_insert_id"`
}

type sqlResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	// FailedStatement is the index of the statement that failed. The handler
	// sets it only when one statement caused the error.
	FailedStatement *int        `json:"failed_statement,omitempty"`
	Results         []sqlResult `json:"results,omitempty"`
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

// evictUserDB closes and forgets a cached handle, thus the next openUserDB
// opens the file again. With isStaleDBHandleError, it lets /api/sql recover
// from an error of the SQLITE_READONLY_DBMOVED class. Without the pair, each
// query fails until a restart. See
// doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md for the
// known cause.
func (a *App) evictUserDB(name string) {
	a.sqlMu.Lock()
	db, ok := a.sqlDBs[name]
	if ok {
		delete(a.sqlDBs, name)
	}
	a.sqlMu.Unlock()
	if ok {
		if err := db.Close(); err != nil {
			a.log(logx.DB).Errf("close evicted handle for %q: %v", name, err)
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

// runSQLBatchWithRetry runs the statements against dbName as one transaction.
// After a stale-handle error, it evicts the handle, opens the database again,
// and runs the batch ONE more time. A retry is safe, because the first
// attempt committed nothing.
func (a *App) runSQLBatchWithRetry(dbName string, statements []sqlStatement) ([]sqlResult, *int, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		db, err := a.openUserDB(dbName)
		if err != nil {
			return nil, nil, err
		}

		tx, err := db.Begin()
		if err != nil {
			if attempt == 1 && isStaleDBHandleError(err) {
				a.log(logx.DB).Infof("%s: stale handle on begin, reopening and retrying: %v", dbName, err)
				a.evictUserDB(dbName)
				lastErr = err
				continue
			}
			return nil, nil, fmt.Errorf("begin: %w", err)
		}

		results := make([]sqlResult, 0, len(statements))
		var failedIdx *int
		var stmtErr error
		for i, stmt := range statements {
			var res sqlResult
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
				a.log(logx.DB).Infof("%s: stale handle on statement #%d, reopening and retrying: %v", dbName, *failedIdx, stmtErr)
				a.evictUserDB(dbName)
				lastErr = stmtErr
				continue
			}
			return nil, failedIdx, stmtErr
		}

		if err := tx.Commit(); err != nil {
			if attempt == 1 && isStaleDBHandleError(err) {
				a.log(logx.DB).Infof("%s: stale handle on commit, reopening and retrying: %v", dbName, err)
				a.evictUserDB(dbName)
				lastErr = err
				continue
			}
			return nil, nil, fmt.Errorf("commit: %w", err)
		}

		return results, nil, nil
	}
	return nil, nil, fmt.Errorf("after retry: %w", lastErr)
}

// handleSQL runs one atomic batch against one named database. See the banner
// for the protocol.
func (a *App) handleSQL(w http.ResponseWriter, r *http.Request) {

	var req sqlRequest
	r.Body = http.MaxBytesReader(w, r.Body, sqlMaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeJSON(w, http.StatusBadRequest, sqlResponse{Status: "error", Message: "bad request: " + err.Error()})
		return
	}
	if len(req.Statements) == 0 {
		a.writeJSON(w, http.StatusBadRequest, sqlResponse{Status: "error", Message: "no statements"})
		return
	}
	if len(req.Statements) > sqlMaxStatements {
		a.writeJSON(w, http.StatusBadRequest, sqlResponse{Status: "error",
			Message: fmt.Sprintf("too many statements (%d > %d)", len(req.Statements), sqlMaxStatements)})
		return
	}

	results, failedIdx, err := a.runSQLBatchWithRetry(req.DB, req.Statements)
	if err != nil {
		a.writeJSON(w, http.StatusBadRequest, sqlResponse{
			Status:          "error",
			Message:         err.Error(),
			FailedStatement: failedIdx,
		})
		return
	}
	a.writeJSON(w, http.StatusOK, sqlResponse{Status: "success", Results: results})
}

func runStatement(tx *sql.Tx, stmt sqlStatement) (sqlResult, error) {
	if !returnsRows(stmt.SQL) {
		res, err := tx.Exec(stmt.SQL, stmt.Args...)
		if err != nil {
			return sqlResult{}, err
		}
		affected, _ := res.RowsAffected()
		lastID, _ := res.LastInsertId()
		return sqlResult{RowsAffected: affected, LastInsertID: lastID}, nil
	}

	rows, err := tx.Query(stmt.SQL, stmt.Args...)
	if err != nil {
		return sqlResult{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return sqlResult{}, err
	}
	out := sqlResult{Columns: cols, Rows: [][]interface{}{}}

	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return sqlResult{}, err
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
