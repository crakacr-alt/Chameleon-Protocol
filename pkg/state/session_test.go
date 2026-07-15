package state

import (
	"testing"
	"time"
)

func TestSessionLifecycleTransitions(t *testing.T) {
	t.Parallel()

	s := NewSession()
	if got := s.State(); got != SessionIdle {
		t.Fatalf("unexpected initial state: %s", got)
	}

	if err := s.Advance(time.Unix(1_700_000_000, 0), "webrtc", time.Minute); err != nil {
		t.Fatalf("advance returned error: %v", err)
	}
	if got := s.State(); got != SessionEstablished {
		t.Fatalf("unexpected state after first advance: %s", got)
	}

	if err := s.Advance(time.Unix(1_700_000_000, 0).Add(time.Minute), "http3", time.Minute); err != nil {
		t.Fatalf("advance returned error: %v", err)
	}
	if got := s.State(); got != SessionEstablished {
		t.Fatalf("unexpected state after epoch rotation: %s", got)
	}
}
