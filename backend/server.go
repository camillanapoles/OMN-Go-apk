package backend

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// App encapsulates the global state for the backend
type App struct {
	config     config.Store
	StorageDir string
	// ActiveConns is an atomic.Int64, and not an int64. A 64-bit atomic needs
	// an 8-byte boundary, and a 32-bit build (armeabi-v7a, x86) does not give
	// one here. Each request then panics. See TestNoBare64BitAtomics.
	ActiveConns atomic.Int64
	GitMutex    sync.Mutex // serializes all on-disk git repo operations
	hostKeys    hostKeyState
	Router      *http.ServeMux

	sqlMu  sync.Mutex         // guards sqlDBs (see sqlite.go)
	sqlDBs map[string]*sql.DB // lazily-opened user SQLite handles, by name

	// dbRestoreMu serializes each database restore and each swap. Never take
	// it while you hold sqlMu.
	dbRestoreMu sync.Mutex

	// search is the global index (search_index.go). It is empty until global
	// search is on.
	search *searchIndex

	// onPageWritten is the hook of renderAndCache. The page cache thus does
	// not call the search index. connectGroups sets the hook, and a nil hook
	// does nothing.
	onPageWritten func(name string)

	// pages holds the values of other groups that each page shows.
	pages pageFacts

	// android holds the facts that the Android layer sets. See androidEnv.
	android androidEnv

	// defaultPort is the port of the flavor, for a config.json with no port.
	// 0 means 8080. Only loadConfig can apply it: see fallbackPort.
	defaultPort int

	ready chan struct{} // closed once the HTTP listener is actually serving

	// boundAddr is what the listener bound, not what the config asked for.
	// /api/status reports it with startedAt. metaMu guards boundAddr.
	metaMu    sync.RWMutex
	startedAt time.Time
	boundAddr string

	// logFilter caches the log switches of the configuration. See
	// applyLogFilter.
	logFilter atomic.Value

	// logs holds the stream clients and the history ring. See logx.Hub.
	logs logx.Hub

	// sessionKey is the HMAC key of the session cookie. It is not a field of
	// Config, because GET /api/config sends the whole Config. See session.go.
	sessionOnce sync.Once
	sessionKey  []byte
}

// WaitUntilReady blocks until the HTTP server listens. It also returns
// when the bind fails, thus a caller never waits for ever.
func (a *App) WaitUntilReady() {
	<-a.ready
}

// connectGroups sets the hooks between the groups. A group thus calls
// another group through the App, and not by name. See group_links_test.go.
func (a *App) connectGroups() {
	a.onPageWritten = func(string) { a.markSearchIndexDirty() }
	a.pages = pageFacts{searchGlobal: a.globalSearchAvailable, incomingPage: incomingIndexName}
}

// runningApp is the App of StartServer. gomobile exports functions only, thus
// an exported setter finds the App here. earlyEnv holds a fact that arrives
// before StartServer: ServerService.java calls SetAndroidPackage first.
var (
	runningMu  sync.Mutex
	runningApp *App
	earlyEnv   androidEnv
)

// setRunningApp gives a the facts of earlyEnv, and each later setter writes
// to a.
func setRunningApp(a *App) {
	runningMu.Lock()
	defer runningMu.Unlock()
	a.android.setPackage(earlyEnv.packageName())
	a.android.setAddresses(earlyEnv.lanAddresses())
	runningApp = a
}

// withAndroidEnv runs fn on the facts of runningApp, or on earlyEnv before
// StartServer.
func withAndroidEnv(fn func(*androidEnv)) {
	runningMu.Lock()
	defer runningMu.Unlock()
	if runningApp != nil {
		fn(&runningApp.android)
		return
	}
	fn(&earlyEnv)
}

// SetAndroidPackage records the applicationId of the Android app.
// ServerService.java calls it before Backend.startServer. gomobile exports
// it.
func SetAndroidPackage(name string) {
	withAndroidEnv(func(e *androidEnv) { e.setPackage(name) })
}

// SetLANAddresses records the addresses of this device as one list with
// commas. ServerService.java calls it with each site-local IPv4 address that
// is not loopback, each time it builds the notification. The notification and
// the Status page thus show the same addresses. The gomobile binding carries
// no slice of strings, thus the value is one string. An empty list clears it.
func SetLANAddresses(list string) {
	out := []string{}
	for _, part := range strings.Split(list, ",") {
		if text := strings.TrimSpace(part); text != "" {
			out = append(out, text)
		}
	}
	withAndroidEnv(func(e *androidEnv) { e.setAddresses(out) })
}

// StartServer starts the Go backend.
//
// A storageDir that is not empty replaces the default of initStorage. Android
// passes the external media directory of its flavor, because the Go runtime
// cannot learn the applicationId. The desktop passes "".
//
// A defaultPort above 0 is the port when config.json has none. The standard
// and the fdroid flavor can run side by side, thus they need two ports. See
// DEFAULT_SERVER_PORT in android/app/build.gradle. The desktop passes 0 for
// 8080.
func StartServer(storageDir string, defaultPort int) *App {
	a := &App{
		Router:    http.NewServeMux(),
		ready:     make(chan struct{}),
		startedAt: time.Now(),
	}

	// Set it before initStorage, which writes config.json on a fresh install.
	a.defaultPort = defaultPort
	setRunningApp(a)

	a.initStorage(storageDir) // Execute synchronously to ensure config is loaded instantly

	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.log(logx.Server).Errf("Recovered from panic in server: %v", r)
			}
		}()

		a.initLogger()
		a.registerRoutes(a.Router)

		// loadConfig already set the port, thus this test is a guard only.
		a.config.Update(func(c *config.Config) {
			if c.ServerPort <= 0 {
				c.ServerPort = a.fallbackPort()
			}
		})
		cfg := a.config.Get()

		// The socket decides who can connect. With "Share on LAN" off, the
		// listener binds the loopback address alone. See
		// doc/decisions/0002-bind-the-loopback-address-when-lan-sharing-is-off.md.
		// The listener binds one time, thus a change applies at the next
		// start.
		bindHost := "127.0.0.1"
		if cfg.ShareLAN {
			bindHost = "0.0.0.0"
		}
		bindAddr := fmt.Sprintf("%s:%d", bindHost, cfg.ServerPort)

		// Bind first, thus WaitUntilReady means "reachable". The bind retries
		// for about 3 seconds: after /api/restart, the new process can reach
		// this line before the old one closes its socket. A port that another
		// program holds still fails.
		var listener net.Listener
		var err error
		for attempt := 1; attempt <= 10; attempt++ {
			listener, err = net.Listen("tcp", bindAddr)
			if err == nil {
				break
			}
			a.log(logx.Server).Debugf("bind %s failed (attempt %d/10), retrying: %v", bindAddr, attempt, err)
			time.Sleep(300 * time.Millisecond)
		}
		if err != nil {
			a.log(logx.Server).Errf("Server failed to bind %s: %v", bindAddr, err)
			close(a.ready) // unblock any waiter rather than hang forever
			return
		}

		// The listener knows the real port, for example after a port of 0.
		a.setBoundAddress(listener.Addr().String())

		a.log(logx.Server).Infof("OMN-Go Backend running on %s", bindAddr)
		close(a.ready)

		if err := http.Serve(listener, a.connectionMiddleware(a.Router)); err != nil {
			a.log(logx.Server).Errf("Server crashed: %v", err)
		}
	}()
	return a
}

// GetServerPort answers the configured port, for main_desktop.go.
func (a *App) GetServerPort() int {
	return a.config.Get().ServerPort
}

// registerRoutes writes each route of the application into mux. Section 3 of
// CLAUDE.md asks for one block.
//
// It takes an interface and not a *http.ServeMux. StartServer passes
// a.Router, and TestBaseline_RouteSet passes a recorder to read the real
// patterns.
type routeTable interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

func (a *App) registerRoutes(mux routeTable) {
	// /api/logs and /api/logs/history are admin only. A remote caller
	// reads no log line, live or held. See handleLogHistory.
	route(mux, "GET", "/api/logs", a.authMiddleware(a.HandleLogsSSE))
	route(mux, "GET", "/api/logs/history", a.authMiddleware(a.handleLogHistory))

	// The catch-all and the asset trees take each method.
	mux.HandleFunc("/", a.serveFrontend)

	// The /js, /css and /json trees hold embedded assets.
	// serveEmbeddableAsset extracts a file at its first request and serves
	// it. ?edit=true opens the editor.
	assetTree := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.serveEmbeddableAsset(w, r, r.URL.Path)
	})
	mux.Handle("/js/", assetTree)
	mux.Handle("/css/", assetTree)
	mux.Handle("/json/", assetTree)

	// /images and /user_json are user content, and the binary embeds none of
	// it. resolveContentType gives each file its type.
	mux.Handle("/images/", a.serveStorageSubdir("images", ""))
	mux.Handle("/user_json/", a.serveStorageSubdir("user_json", ""))

	route(mux, "POST", "/login", a.handleLogin)
	route(mux, "POST", "/api/quick", a.authMiddleware(a.handleQuickNote))
	route(mux, "POST", "/api/bookmark", a.authMiddleware(a.handleBookmark))
	route(mux, "POST", "/api/upload", a.authMiddleware(a.handleUpload))
	route(mux, "POST", "/api/upload_json", a.authMiddleware(a.handleUploadJSON))
	route(mux, "GET", "/api/note", a.handleGetNote)
	// This route has no authMiddleware, the same as /api/note and each page.
	// Search collects nothing that a remote caller cannot read file by file.
	route(mux, "GET", "/api/search", a.handleSearch)
	route(mux, "POST", "/api/save", a.authMiddleware(a.handleSaveNote))
	route(mux, "POST", "/api/newpage", a.authMiddleware(a.handleNewPage))
	mux.HandleFunc("GET /api/config", a.authMiddleware(a.handleConfigGet))
	mux.HandleFunc("POST /api/config", a.authMiddleware(a.handleConfigPost))
	mux.HandleFunc("/api/config", refuseMethod("GET", "POST"))
	route(mux, "POST", "/api/restart", a.authMiddleware(a.handleRestart))
	route(mux, "POST", "/api/sql", a.authMiddleware(a.handleSQL))
	route(mux, "POST", "/api/db/backup", a.authMiddleware(a.handleDBBackupCreate))
	route(mux, "GET", "/api/db/backups", a.authMiddleware(a.handleDBBackupList))
	route(mux, "POST", "/api/db/restore", a.authMiddleware(a.handleDBRestore))
	route(mux, "POST", "/api/sync", a.authMiddleware(a.handleSync))
	route(mux, "GET", "/api/sync/preview", a.authMiddleware(a.handleSyncPreview))
	route(mux, "POST", "/api/sync/trust-host-key", a.authMiddleware(a.handleTrustHostKey))
	route(mux, "GET", "/api/edit-external", a.authMiddleware(a.handleEditExternal))
	// Note exchange. Both routes are admin only: import writes files, and
	// export is a way out of the note tree. The device itself is always
	// admin, and on Android the device is the caller.
	route(mux, "GET", "/api/export/note", a.authMiddleware(a.handleExportNote))
	route(mux, "POST", "/api/import/note", a.authMiddleware(a.handleImportNote))
	// This route is admin only, because the answer holds LAN addresses,
	// absolute paths and a commit subject.
	route(mux, "GET", "/api/status", a.authMiddleware(a.handleStatus))
	// The loop registers each system page. See page_access.go.
	for _, p := range a.systemPages() {
		route(mux, "GET", p.path, a.pageHandler(p))
	}
}

// route registers h for one method on one path. The pattern "GET /x" also
// takes HEAD. The bare path answers 405 for another method. See
// doc/decisions/0016-give-each-route-one-method.md.
func route(mux routeTable, method, path string, h http.HandlerFunc) {
	mux.HandleFunc(method+" "+path, h)
	mux.HandleFunc(path, refuseMethod(method))
}

// refuseMethod answers 405, and the Allow header names the methods.
func refuseMethod(methods ...string) http.HandlerFunc {
	var allow []string
	for _, m := range methods {
		allow = append(allow, m)
		if m == http.MethodGet {
			allow = append(allow, http.MethodHead)
		}
	}
	header := strings.Join(allow, ", ")
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", header)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
