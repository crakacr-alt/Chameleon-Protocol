package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Route represents a learned routing decision for a destination.
type Route struct {
	Destination string `json:"destination"`
	Profile     string `json:"profile"`
	LastUsed    int64  `json:"last_used"`
}

// Store persists a small set of learned routes to disk.
type Store struct {
	mu     sync.Mutex
	Path   string           `json:"path"`
	Routes map[string]Route `json:"routes"`
}

// NewStore opens or creates a route store.
func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir store dir: %w", err)
	}
	s := &Store{Path: path, Routes: make(map[string]Route)}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("unmarshal routes: %w", err)
		}
		if s.Routes == nil {
			s.Routes = make(map[string]Route)
		}
	}
	return s, nil
}

// Save persists current routes to disk.
func (s *Store) Save() error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal routes: %w", err)
	}
	if err := os.WriteFile(s.Path, payload, 0o644); err != nil {
		return fmt.Errorf("write routes: %w", err)
	}
	return nil
}

// Upsert records or updates a route for a destination.
func (s *Store) Upsert(dst, profile string, lastUsed int64) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if dst == "" {
		return fmt.Errorf("destination must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Routes[dst] = Route{Destination: dst, Profile: profile, LastUsed: lastUsed}
	return s.Save()
}

// Get returns the route for the destination if known.
func (s *Store) Get(dst string) (Route, bool) {
	if s == nil {
		return Route{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.Routes[dst]
	return r, ok
}
