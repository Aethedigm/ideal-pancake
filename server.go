package main

import "sync"

type Server struct {
	URL    string `json:"url"`
	isDead bool
	mutex  sync.RWMutex
}

// Set state
func (s *Server) Dead(b bool) {
	s.mutex.Lock()
	s.isDead = b
	s.mutex.Unlock()
}

func (s *Server) GetIsDead() bool {
	// Read locks do not stop other read locks
	s.mutex.RLock()
	// Value copy out current state
	isDead := s.isDead
	s.mutex.RUnlock()

	// Return value copy
	return isDead
}
