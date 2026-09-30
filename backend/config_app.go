package backend

import (
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// The App side of the config package. Each method reads the config.Store of
// the App, or writes the cache of the log switches.

// loadConfig reads configPath, the config.json of storage.Layout.Config.
func (a *App) loadConfig(configPath string) {
	a.config.Update(func(c *config.Config) {
		config.Load(c, configPath, a.fallbackPort(), a.log(logx.Config))
		// Call this last. Each line of load is a fault, and a fault always
		// prints. See applyLogFilter.
		a.applyLogFilter(*c)
	})
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

// applyLogFilter caches the log switches of one configuration.
//
// A LOG LINE MUST NEVER TAKE THE CONFIG LOCK. loadConfig holds the write lock
// and can write a log line, and a Go RWMutex is not reentrant. A read of the
// config from emit would thus deadlock the start. An atomic value costs
// one load for each line. loadConfig and handleConfigPost refresh the cache.
func (a *App) applyLogFilter(c config.Config) {
	a.logFilter.Store(config.LogFilter(c))
}

// maxUploadBytes answers the upload limit of the configuration in bytes.
func (a *App) maxUploadBytes() int64 { return config.MaxUploadBytes(a.config.Get()) }

// resolveContentType answers the content type of path. See
// doc/decisions/0003-use-one-table-for-each-content-type.md.
func (a *App) resolveContentType(path string) string {
	return config.ResolveContentType(a.config.Get().MimeTypes, path)
}

// hasKnownAssetExtension tells if name is a file under html/, and not a note.
func (a *App) hasKnownAssetExtension(name string) bool {
	return config.HasKnownAssetExtension(a.config.Get().MimeTypes, name)
}
