package config

import "sync"

// Store holds the configuration under one lock. Get gives a copy, and
// Update changes the configuration under the write lock. A function that
// gets the store needs no *App to read the configuration.
//
// A LOG LINE MUST NEVER TAKE THIS LOCK. The loadConfig method of package
// backend holds the write lock and writes log lines. See applyLogFilter
// in backend/config_app.go.
type Store struct {
	mu  sync.RWMutex
	cfg Config
}

// Get answers a copy of the configuration.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Update runs fn under the write lock, for a read, a change and a write
// together.
func (s *Store) Update(fn func(c *Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
}
