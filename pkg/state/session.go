package state

import (
	"fmt"
	"sync"
	"time"
)

// SessionState is the explicit lifecycle state for a transport session.
type SessionState string

const (
	SessionIdle        SessionState = "idle"
	SessionKeyExchange SessionState = "key_exchange"
	SessionEstablished SessionState = "established"
	SessionRotating    SessionState = "rotating"
	SessionClosed      SessionState = "closed"
)

// Session tracks a transport session lifecycle across epochs.
type Session struct {
	mu             sync.Mutex
	state          SessionState
	epoch          int64
	profile        string
	lastTransition time.Time
}

// SecurityContext holds epoch-bound security artifacts for a session.
type SecurityContext struct {
	EpochID       []byte
	SymmetricKey  []byte // derived per-epoch
	EntropyBudget int64  // remaining entropy budget in bytes
}

// Session now embeds a SecurityContext for key lifecycle management.
type SessionWithSecurity struct {
	*Session
	Sec *SecurityContext
}

// NewSession creates a new stateful transport session.
func NewSession() *Session {
	return &Session{state: SessionIdle, profile: "webrtc"}
}

// NewSessionWithSecurity creates a session prepopulated with a security context placeholder.
func NewSessionWithSecurity() *SessionWithSecurity {
	return &SessionWithSecurity{Session: NewSession(), Sec: &SecurityContext{EntropyBudget: 1024}}
}

// State returns the current session lifecycle state.
func (s *Session) State() SessionState {
	if s == nil {
		return SessionClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == "" {
		return SessionIdle
	}
	return s.state
}

// Epoch returns the current epoch tracked by the session.
func (s *Session) Epoch() int64 {
	if s == nil {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}

// Begin starts the session lifecycle with explicit key-exchange state.
func (s *Session) Begin() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.state {
	case SessionClosed:
		return fmt.Errorf("session is closed")
	case SessionIdle, "":
		s.state = SessionKeyExchange
		s.lastTransition = time.Now()
		return nil
	default:
		return nil
	}
}

// Advance moves the session through established and rotating transitions.
func (s *Session) Advance(ts time.Time, profile string, period time.Duration) error {
	if s == nil {
		return nil
	}
	if profile == "" {
		profile = "webrtc"
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

	switch s.state {
	case SessionClosed:
		return fmt.Errorf("session is closed")
	case "", SessionIdle:
		s.state = SessionKeyExchange
		s.lastTransition = ts
	case SessionKeyExchange:
		s.state = SessionEstablished
		s.lastTransition = ts
	}

	if s.epoch != epoch || s.profile != profile {
		s.state = SessionRotating
		s.lastTransition = ts
		s.epoch = epoch
		s.profile = profile
		s.state = SessionEstablished
	}

	return nil
}

// Close terminates the session and prevents further transitions.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = SessionClosed
	s.lastTransition = time.Now()
	return nil
}
