package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// SessionContext captures a transport session identity, active profile, and
// epoch-bound key material for a controlled rekey policy.
type SessionContext struct {
	mu        sync.Mutex
	sessionID string
	profile   string
	rootKey   []byte
	epoch     int64
	key       []byte
}

// NewSessionContext creates a new session context with a stable root key.
func NewSessionContext(sessionID, profile string, rootKey []byte) *SessionContext {
	if profile == "" {
		profile = "webrtc"
	}
	if len(rootKey) == 0 {
		rootKey = []byte("chameleon-root")
	}

	return &SessionContext{
		sessionID: sessionID,
		profile:   profile,
		rootKey:   append([]byte(nil), rootKey...),
	}
}

// Profile returns the currently active profile.
func (s *SessionContext) Profile() string {
	if s == nil {
		return "webrtc"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.profile == "" {
		return "webrtc"
	}
	return s.profile
}

// Epoch returns the current epoch number tracked by the context.
func (s *SessionContext) Epoch() int64 {
	if s == nil {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}

// Rotate derives a per-epoch key from the root key and returns the new epoch key.
func (s *SessionContext) Rotate(ts time.Time, period time.Duration) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("session context is nil")
	}
	if period <= 0 {
		period = time.Minute
	}

	epoch := ts.UnixNano() / int64(period)
	if epoch < 0 {
		epoch = 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	msg := []byte(fmt.Sprintf("%s:%d", s.sessionID, epoch))
	mac := hmac.New(sha256.New, s.rootKey)
	if _, err := mac.Write(msg); err != nil {
		return nil, fmt.Errorf("write hmac input: %w", err)
	}
	derived := mac.Sum(nil)
	encoded := []byte(hex.EncodeToString(derived))

	s.epoch = epoch
	s.key = encoded
	return append([]byte(nil), encoded...), nil
}
