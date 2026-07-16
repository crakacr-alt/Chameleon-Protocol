package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// Sync provides an epoch-aware key rotation helper that maps a PSK into
// a deterministic profile and exposes the derived epoch ID for use in
// key derivation. It reuses a HMAC-based seed for deterministic behavior.
type Sync struct {
	mu         sync.Mutex
	PSK        string
	EpochPeriod time.Duration
	lastEpoch  int64
	profile    string
}

// NewSync creates a Sync controller bound to a shared secret.
func NewSync(psk string) *Sync {
	return &Sync{PSK: psk, EpochPeriod: time.Minute}
}

// ProfileAt returns the profile name for the provided time according to the PSK.
func (s *Sync) ProfileAt(ts time.Time) string {
	if s == nil {
		return "webrtc"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	period := s.EpochPeriod
	if period <= 0 {
		period = time.Minute
	}
	epoch := ts.UnixNano() / int64(period)

	if epoch == s.lastEpoch && s.profile != "" {
		return s.profile
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(epoch))
	mac := hmac.New(sha256.New, []byte(s.PSK))
	if _, err := mac.Write(buf); err != nil {
		// fallback deterministic
		s.profile = "webrtc"
		s.lastEpoch = epoch
		return s.profile
	}
	seed := mac.Sum(nil)
	order := []string{"webrtc", "http3", "gaming"}
	idx := int(seed[0]) % len(order)
	s.profile = order[(int(epoch)+idx)%len(order)]
	s.lastEpoch = epoch
	return s.profile
}

// EpochID returns a compact epoch identifier for key derivation.
func (s *Sync) EpochID(ts time.Time) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("sync is nil")
	}
	period := s.EpochPeriod
	if period <= 0 {
		period = time.Minute
	}
	epoch := ts.UnixNano() / int64(period)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(epoch))
	mac := hmac.New(sha256.New, []byte(s.PSK))
	if _, err := mac.Write(buf); err != nil {
		return nil, fmt.Errorf("compute epoch id: %w", err)
	}
	return mac.Sum(nil), nil
}
