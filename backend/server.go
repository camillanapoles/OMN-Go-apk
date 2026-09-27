package backend

import (
	"database/sql"
	"embed"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// App encapsulates the global state for the backend
type App struct {
	Config      Config
	ConfigMutex sync.RWMutex // guards all reads/writes of Config
	StorageDir  string
	// ActiveConns is an atomic.Int64, and not an int64. A 64-bit atomic needs
	// an 8-byte boundary, and a 32-bit build (armeabi-v7a, x86) does not give
	// one here. Each request then panics. See TestNoBare64BitAtomics.
	ActiveConns atomic.Int64
	GitMutex    sync.Mutex // serializes all on-disk git repo operations
	Router      *http.ServeMux

	sqlMu  sync.Mutex         // guards sqlDBs (see sqlite.go)
	sqlDBs map[string]*sql.DB // lazily-opened user SQLite handles, by name

	// dbRestoreMu serializes each database restore and each swap. Never take
	// it while you hold sqlMu.
	dbRestoreMu sync.Mutex

	// search is the global index (search_index.go). It is empty until global
	// search is on.
	search *searchIndex

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

	// sessionKey is the HMAC key of the session cookie. It is not a field of
	// Config, because GET /api/config sends the whole Config. See session.go.
	sessionOnce sync.Once
	sessionKey  []byte
}

// boundAddress reports the address of the listener as host, port and the
// joined form. Each value is empty before the bind.
func (a *App) boundAddress() (host, port, addr string) {
	a.metaMu.RLock()
	addr = a.boundAddr
	a.metaMu.RUnlock()
	if addr == "" {
		return "", "", ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", addr
	}
	return host, port, addr
}

func (a *App) setBoundAddress(addr string) {
	a.metaMu.Lock()
	a.boundAddr = addr
	a.metaMu.Unlock()
}

// fallbackPort is the port for a config.json with none. Only the config
// loader can apply it. loadConfig writes the port into config.json on a fresh
// install, and a later default would never reach the file. See
// DEFAULT_SERVER_PORT in android/app/build.gradle.
func (a *App) fallbackPort() int {
	if a.defaultPort > 0 {
		return a.defaultPort
	}
	return 8080
}

// GetConfig returns a copy of the current config, safe for concurrent reads.
func (a *App) GetConfig() Config {
	a.ConfigMutex.RLock()
	defer a.ConfigMutex.RUnlock()
	return a.Config
}

// WithConfig runs fn under the config write lock, for a read, a change and a
// write together.
func (a *App) WithConfig(fn func(c *Config)) {
	a.ConfigMutex.Lock()
	defer a.ConfigMutex.Unlock()
	fn(&a.Config)
}

// WaitUntilReady blocks until the HTTP server listens. It also returns
// when the bind fails, thus a caller never waits for ever.
func (a *App) WaitUntilReady() {
	<-a.ready
}

//go:embed frontend/html frontend/md
var staticFS embed.FS

// templatesFS holds the page fragments that the server renders. It is apart
// from staticFS on purpose. The files of staticFS reach StorageDir/html, and
// a person can edit them with ?edit=true. A template is render logic, and a
// person must not damage it.
//
//go:embed frontend/templates
var templatesFS embed.FS

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

	a.initStorage(storageDir) // Execute synchronously to ensure config is loaded instantly

	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logErrf(logServer, "Recovered from panic in server: %v", r)
			}
		}()

		a.initLogger()
		a.registerRoutes(a.Router)

		// This access needs no lock: no handler runs before net.Listen.
		// loadConfig already set the port, thus this test is a guard only.
		if a.Config.ServerPort <= 0 {
			a.Config.ServerPort = a.fallbackPort()
		}

		// The socket decides who can connect. With "Share on LAN" off, the
		// listener binds the loopback address alone. See
		// doc/decisions/0002-bind-the-loopback-address-when-lan-sharing-is-off.md.
		// The listener binds one time, thus a change applies at the next
		// start.
		bindHost := "127.0.0.1"
		if a.Config.ShareLAN {
			bindHost = "0.0.0.0"
		}
		bindAddr := fmt.Sprintf("%s:%d", bindHost, a.Config.ServerPort)

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
			a.logDebugf(logServer, "bind %s failed (attempt %d/10), retrying: %v", bindAddr, attempt, err)
			time.Sleep(300 * time.Millisecond)
		}
		if err != nil {
			a.logErrf(logServer, "Server failed to bind %s: %v", bindAddr, err)
			close(a.ready) // unblock any waiter rather than hang forever
			return
		}

		// The listener knows the real port, for example after a port of 0.
		a.setBoundAddress(listener.Addr().String())

		a.logInfof(logServer, "OMN-Go Backend running on %s", bindAddr)
		close(a.ready)

		if err := http.Serve(listener, a.connectionMiddleware(a.Router)); err != nil {
			a.logErrf(logServer, "Server crashed: %v", err)
		}
	}()
	return a
}

// GetServerPort answers the configured port, for main_desktop.go.
func (a *App) GetServerPort() int {
	return a.GetConfig().ServerPort
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
	// /api/logs and /api/logs/history are admin only. A guest on the LAN
	// reads no log line, live or held. See handleLogHistory.
	mux.HandleFunc("/api/logs", a.authMiddleware(a.HandleLogsSSE))

	mux.HandleFunc("/api/logs/history", a.authMiddleware(a.handleLogHistory))
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
	// it.
	// resolveContentType gives each file its type.
	mux.Handle("/images/", a.serveStorageSubdir("images", ""))
	mux.Handle("/user_json/", a.serveStorageSubdir("user_json", ""))

	mux.HandleFunc("/login", a.handleLogin)
	mux.HandleFunc("/api/quick", a.authMiddleware(a.handleQuickNote))
	mux.HandleFunc("/api/bookmark", a.authMiddleware(a.handleBookmark))
	mux.HandleFunc("/api/upload", a.authMiddleware(a.handleUpload))
	mux.HandleFunc("/api/upload_json", a.authMiddleware(a.handleUploadJSON))
	mux.HandleFunc("/api/note", a.handleGetNote)
	// This route has no authMiddleware, the same as /api/note and each page.
	// Search collects nothing that a guest cannot read file by file.
	mux.HandleFunc("/api/search", a.handleSearch)
	mux.HandleFunc("/api/save", a.authMiddleware(a.handleSaveNote))
	mux.HandleFunc("/api/newpage", a.authMiddleware(a.handleNewPage))
	mux.HandleFunc("/api/config", a.authMiddleware(a.handleConfig))
	mux.HandleFunc("/api/restart", a.authMiddleware(a.handleRestart))
	mux.HandleFunc("/api/sql", a.authMiddleware(a.handleSQL))
	mux.HandleFunc("/api/db/backup", a.authMiddleware(a.handleDBBackupCreate))
	mux.HandleFunc("/api/db/backups", a.authMiddleware(a.handleDBBackupList))
	mux.HandleFunc("/api/db/restore", a.authMiddleware(a.handleDBRestore))
	mux.HandleFunc("/db_backups", a.authMiddleware(a.serveDBBackupsPage))
	// This is a PAGE with its own route, because the catch-all needs no login
	// and this listing is admin only. The handler asks hasRole itself, thus a
	// refusal is a page and not a line of text.
	mux.HandleFunc("/OMNGoFiles.html", a.serveFilesPage)
	mux.HandleFunc("/api/sync", a.authMiddleware(a.handleSync))
	mux.HandleFunc("/api/sync/preview", a.authMiddleware(a.handleSyncPreview))
	mux.HandleFunc("/api/edit-external", a.authMiddleware(a.handleEditExternal))
	// Note exchange. Both routes are admin only: import writes files, and
	// export is a way out of the note tree. The device itself is always
	// admin, and on Android the device is the caller.
	mux.HandleFunc("/api/export/note", a.authMiddleware(a.handleExportNote))
	mux.HandleFunc("/api/import/note", a.authMiddleware(a.handleImportNote))
	// This route is admin only, because the answer holds LAN addresses,
	// absolute paths and a commit subject.
	mux.HandleFunc("/api/status", a.authMiddleware(a.handleStatus))
	// The Status page and the Log page ask hasRole themselves, the same as
	// /OMNGoFiles.html.
	mux.HandleFunc("/OMNGoStatus.html", a.serveStatusPage)
	mux.HandleFunc("/OMNGoLogs.html", a.serveLogsPage)
}
