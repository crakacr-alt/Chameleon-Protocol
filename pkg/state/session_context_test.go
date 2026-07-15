package state

import (
	"testing"
	"time"
)

func TestSessionContextRotatesKeyPerEpoch(t *testing.T) {
	t.Parallel()

	ctx := NewSessionContext("session-1", "webrtc", []byte("root-secret"))
	first, err := ctx.Rotate(time.Unix(1_700_000_000, 0), time.Minute)
	if err != nil {
		t.Fatalf("Rotate returned error: %v", err)
	}
	second, err := ctx.Rotate(time.Unix(1_700_000_000, 0).Add(time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("Rotate returned error: %v", err)
	}

	if string(first) == string(second) {
		t.Fatalf("epoch key should rotate when epoch changes")
	}
	if ctx.Profile() != "webrtc" {
		t.Fatalf("unexpected profile after rotation: %s", ctx.Profile())
	}
}

func TestSessionContextReportsEpochState(t *testing.T) {
	t.Parallel()

	ctx := NewSessionContext("session-1", "webrtc", []byte("root-secret"))
	_, err := ctx.Rotate(time.Unix(1_700_000_000, 0), time.Minute)
	if err != nil {
		t.Fatalf("Rotate returned error: %v", err)
	}

	if got := ctx.Epoch(); got != 1_700_000_000_000_000_000/time.Minute.Nanoseconds() {
		t.Fatalf("unexpected epoch value: %d", got)
	}
}
