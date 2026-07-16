package identity

import (
	"encoding/json"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"crypto/ed25519"
	"crypto/rand"
)

// Store provides a tiny identity registry persisted as JSON.
type Store struct {
	mu    sync.Mutex
	Path  string            `json:"path"`
	IdMap map[string]string `json:"id_map"`
}

// LoadOrCreateIdentity loads an Ed25519 private key from disk or creates one
// and returns the base64-encoded public key.
func (s *Store) LoadOrCreateIdentity(id string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("store is nil")
	}
	if id == "" {
		return "", fmt.Errorf("id must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// file path is store dir + id.key
	keyPath := filepath.Join(filepath.Dir(s.Path), id+".key")
	if data, err := os.ReadFile(keyPath); err == nil && len(data) > 0 {
		// data is base64-encoded private key
		raw, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return "", fmt.Errorf("decode private key: %w", err)
		}
		// public key is last 32 bytes of ed25519 private key
		if len(raw) >= 32 {
			pub := raw[len(raw)-32:]
			return base64.StdEncoding.EncodeToString(pub), nil
		}
		return "", fmt.Errorf("invalid private key size")
	}

	// Create identity by delegating to KeyManager if available.
	// For now, generate a new ed25519 key and save it as base64 private key.
	// Note: this is a minimal bootstrap; in production use KeyManager.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate ed25519 key: %w", err)
	}
	enc := base64.StdEncoding.EncodeToString([]byte(priv))
	if err := os.WriteFile(keyPath, []byte(enc), 0o600); err != nil {
		return "", fmt.Errorf("write identity key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

// NewStore initializes or loads a store from disk.
func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("store path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir store dir: %w", err)
	}
	s := &Store{Path: path, IdMap: make(map[string]string)}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("unmarshal store: %w", err)
		}
		if s.IdMap == nil {
			s.IdMap = make(map[string]string)
		}
	}
	return s, nil
}

// Register adds or updates an identity mapping and persists the store.
func (s *Store) Register(id, pub string) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if id == "" || pub == "" {
		return fmt.Errorf("id and pub must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IdMap[id] = pub
	return s.saveLocked()
}

// Lookup returns the stored public key for an identity.
func (s *Store) Lookup(id string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.IdMap[id]
	return v, ok
}

func (s *Store) saveLocked() error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	if err := os.WriteFile(s.Path, payload, 0o644); err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	return nil
}
