package state

import (
	"fmt"
	"sync"
	"time"
)

// RekeyPolicy configures when a rekey should be triggered.
type RekeyPolicy struct {
	Period    time.Duration // periodic rekey interval
	MaxBytes  int64         // optional: rekey after this many bytes transmitted
	FailCount int           // optional: rekey after N consecutive failures
}

// RekeyState tracks counters and decides when to trigger rekey.
type RekeyState struct {
	mu             sync.Mutex
	policy         RekeyPolicy
	lastRekey      time.Time
	bytesSent      int64
	consecutiveErr int
}

// NewRekeyState creates a RekeyState with a policy.
func NewRekeyState(policy RekeyPolicy) *RekeyState {
	if policy.Period <= 0 {
		policy.Period = time.Minute
	}
	return &RekeyState{policy: policy, lastRekey: time.Now()}
}

// RecordSend updates counters for sent bytes and returns whether rekey is needed.
func (r *RekeyState) RecordSend(bytes int) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bytesSent += int64(bytes)
	if r.policy.MaxBytes > 0 && r.bytesSent >= r.policy.MaxBytes {
		return true
	}
	if time.Since(r.lastRekey) >= r.policy.Period {
		return true
	}
	return false
}

// RecordFailure updates failure counter and returns whether rekey is needed.
func (r *RekeyState) RecordFailure() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consecutiveErr++
	if r.policy.FailCount > 0 && r.consecutiveErr >= r.policy.FailCount {
		return true
	}
	return false
}

// Reset resets counters after a successful rekey or recovery.
func (r *RekeyState) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.bytesSent = 0
	r.consecutiveErr = 0
	r.lastRekey = time.Now()
	r.mu.Unlock()
}

// NextRekeyTime returns approximate next rekey time according to policy.
func (r *RekeyState) NextRekeyTime() (time.Time, error) {
	if r == nil {
		return time.Time{}, fmt.Errorf("rekey state nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastRekey.Add(r.policy.Period), nil
}
