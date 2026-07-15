package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// EpochState implements a deterministic, bounded profile rotation machine.
type EpochState struct {
	mu        sync.Mutex
	period    time.Duration
	lastEpoch int64
	profile   string
}

// NewEpochState creates a stateful epoch controller.
func NewEpochState(period time.Duration) *EpochState {
	if period <= 0 {
		period = time.Minute
	}

	return &EpochState{period: period}
}

// Current returns the deterministic profile for the given timestamp.
func (s *EpochState) Current(ts time.Time) (string, error) {
	if s == nil {
		return "webrtc", nil
	}

	period := s.period
	if period <= 0 {
		period = time.Minute
	}

	epoch := ts.UnixNano() / int64(period)
	if epoch < 0 {
		epoch = 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if epoch == s.lastEpoch && s.profile != "" {
		return s.profile, nil
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(epoch))

	mac := hmac.New(sha256.New, []byte("chameleon-epoch"))
	if _, err := mac.Write(buf); err != nil {
		return "", fmt.Errorf("write hmac input: %w", err)
	}
	seed := mac.Sum(nil)

	profileOrder := []string{"webrtc", "http3", "gaming"}
	start := int(seed[0]) % len(profileOrder)
	index := (int(epoch) + start) % len(profileOrder)

	s.profile = profileOrder[index]
	s.lastEpoch = epoch

	return s.profile, nil
}
