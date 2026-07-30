package appstate

import (
	"database/sql"
	"sync"
)

// State holds the live app DB connection behind a lock so it can be set
// once the Setup Wizard finishes — without it, every handler would keep
// referencing the nil connection captured at process boot and the app
// would need a manual restart right after setup completes.
type State struct {
	mu sync.RWMutex
	db *sql.DB
}

func New(db *sql.DB) *State {
	return &State{db: db}
}

func (s *State) DB() *sql.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db
}

func (s *State) SetDB(db *sql.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
}
