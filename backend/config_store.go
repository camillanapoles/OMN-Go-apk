package backend

import "sync"

// configStore holds the configuration under one lock. get gives a copy, and
// update changes the configuration under the write lock. A function that
// gets the store needs no *App to read the configuration.
//
// A LOG LINE MUST NEVER TAKE THIS LOCK. loadConfig holds the write lock and
// writes log lines. See applyLogFilter.
type configStore struct {
	mu  sync.RWMutex
	cfg Config
}

// get answers a copy of the configuration.
func (s *configStore) get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// update runs fn under the write lock, for a read, a change and a write
// together.
func (s *configStore) update(fn func(c *Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
}
