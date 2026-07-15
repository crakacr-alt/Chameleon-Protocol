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

// NewSession creates a new stateful transport session.
func NewSession() *Session {
	return &Session{state: SessionIdle, profile: "webrtc"}
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

// Advance moves the session forward through the lifecycle and records epoch changes.
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
	case "", SessionIdle:
		s.state = SessionKeyExchange
	case SessionKeyExchange:
		s.state = SessionEstablished
	case SessionRotating:
		s.state = SessionEstablished
	case SessionClosed:
		return fmt.Errorf("session is closed")
	}

	if s.epoch != epoch || s.profile != profile {
		s.state = SessionRotating
		s.epoch = epoch
		s.profile = profile
		s.lastTransition = ts
		s.state = SessionEstablished
	}

	return nil
}
