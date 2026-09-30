package app

import (
	"log"

	"net.basov.omngo/backend/internal/logx"
)

// log gives the logger of one tag.
func (a *App) log(tag logx.Tag) logx.Logger {
	return logx.New(tag, &a.logFilter, &a.logs)
}

// logLineEnabled tells whether one line reaches stdout and the browser
// console. An error always does. A debug or info line needs its level on and
// its tag checked. Before loadConfig runs, the cache is empty and allows
// faults only, the same as a fresh install.
func (a *App) logLineEnabled(lvl logx.Level, tag logx.Tag) bool {
	return a.log(tag).Enabled(lvl)
}

// initLogger sends the standard logger into the /api/logs stream.
// registerRoutes in server.go registers the route. The function is not
// exported. See section 3 of CLAUDE.md for the exported names.
func (a *App) initLogger() {
	log.SetOutput(a.logs.StdWriter())
}
