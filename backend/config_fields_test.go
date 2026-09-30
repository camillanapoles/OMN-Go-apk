package backend

// Tests of the settings that need an App: the rebuild of the search index
// and the git slot array. internal/config/fields_test.go tests the table of
// settings.

import (
	"testing"

	"net.basov.omngo/backend/internal/config"
)

// ----------------------------------------------------------------------
// The work that a saved change starts
// ----------------------------------------------------------------------

// A rebuild of the global index reads each note, thus a save must start
// one only when the index is really wrong.
//
// The caller tests SearchEnabled before it calls this, thus each row
// below has search on in the new configuration.
func TestSearchIndexNeedsRebuild(t *testing.T) {
	on := func(f func(*config.Config)) config.Config {
		c := config.Config{SearchEnabled: true, SearchKinds: []string{"md", "bookmarks"}}
		if f != nil {
			f(&c)
		}
		return c
	}
	for _, tt := range []struct {
		what string
		prev config.Config
		next config.Config
		want bool
	}{
		{"search was off", on(func(c *config.Config) { c.SearchEnabled = false }), on(nil), true},
		{"nothing changed", on(nil), on(nil), false},
		{"the kinds changed", on(nil), on(func(c *config.Config) { c.SearchKinds = []string{"md"} }), true},
		{"the bundled switch changed", on(nil), on(func(c *config.Config) { c.SearchBundled = true }), true},
		{
			// A nil list and the default list are the same set. A save
			// that writes the default over a nil must not rebuild.
			"nil against the default list",
			on(func(c *config.Config) { c.SearchKinds = nil }),
			on(func(c *config.Config) { c.SearchKinds = config.NormalizeSearchKinds(nil) }),
			false,
		},
		{
			// A change that no part of the index reads.
			"an unrelated field changed",
			on(nil), on(func(c *config.Config) { c.Author = "Ann" }), false,
		},
	} {
		if got := searchIndexNeedsRebuild(tt.prev, tt.next); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.what, got, tt.want)
		}
	}
}

// ----------------------------------------------------------------------
// The git slot array
// ----------------------------------------------------------------------

// loadConfig must always leave config.MaxGitServers slots, whatever the file
// holds. Each renderer and each handler indexes that array by number, and
// a short array is an out-of-range panic waiting for a save.
func TestConfigWithFewGitServersIsPadded(t *testing.T) {
	for _, tt := range []struct{ what, file string }{
		{"no key at all", `{"author":"Ann"}`},
		{"an explicit null", `{"git_servers":null}`},
		{"an empty array", `{"git_servers":[]}`},
		{"one slot", `{"git_servers":[{"name":"mine","url":"git@host:r.git"}]}`},
		{"a full array", `{"git_servers":[{},{},{},{},{}]}`},
	} {
		a := newUnconfiguredApp(t)
		writeConfigJSON(t, a, tt.file)
		a.loadConfig(a.layout().config())

		cfg := a.config.Get()
		if len(cfg.GitServers) != config.MaxGitServers {
			t.Errorf("%s: %d slots, want %d", tt.what, len(cfg.GitServers), config.MaxGitServers)
			continue
		}
		// A slot that the file carried keeps its values. Padding must
		// add rows and never rewrite one.
		if tt.what == "one slot" {
			if cfg.GitServers[0].Name != "mine" || cfg.GitServers[0].URL != "git@host:r.git" {
				t.Errorf("padding overwrote the slot the file carried: %+v", cfg.GitServers[0])
			}
			// Each added row carries the label that the Config page shows
			// for an empty slot.
			if cfg.GitServers[4].Name != "Server 5" {
				t.Errorf("added slot 5 is named %q, want %q", cfg.GitServers[4].Name, "Server 5")
			}
		}
	}
}

// A fresh install gets the same array. The branch that writes the default
// configuration does not pad, thus the one loop after both branches is
// what covers it.
func TestFreshInstallHasEveryGitSlot(t *testing.T) {
	a := newUnconfiguredApp(t)
	a.loadConfig(a.layout().config())
	if got := len(a.config.Get().GitServers); got != config.MaxGitServers {
		t.Errorf("a fresh install has %d slots, want %d", got, config.MaxGitServers)
	}
}
