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

	if err := s.Begin(); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if got := s.State(); got != SessionKeyExchange {
		t.Fatalf("unexpected state after Begin: %s", got)
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

func TestSessionCloseRejectsFurtherAdvances(t *testing.T) {
	t.Parallel()

	s := NewSession()
	if err := s.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := s.Advance(time.Now(), "http3", time.Minute); err == nil {
		t.Fatal("Advance should fail after Close")
	}
}
