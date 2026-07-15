package state

import (
	"testing"
	"time"
)

func TestProfileAtIsStable(t *testing.T) {
	t.Parallel()

	s := NewSync("shared-secret")
	ts := time.Unix(1_700_000_000, 0)
	first := s.ProfileAt(ts)
	second := s.ProfileAt(ts.Add(time.Second))

	if first == "" {
		t.Fatal("profile should not be empty")
	}
	if first != second {
		t.Fatalf("profile should be stable within the same epoch; got %q and %q", first, second)
	}
}

func TestEpochStateMachineRotatesCleanly(t *testing.T) {
	t.Parallel()

	state := NewEpochState(time.Minute)
	start := time.Unix(1_700_000_000, 0)

	first, err := state.Current(start)
	if err != nil {
		t.Fatalf("Current returned error: %v", err)
	}
	second, err := state.Current(start.Add(time.Second))
	if err != nil {
		t.Fatalf("Current returned error: %v", err)
	}
	if first != second {
		t.Fatalf("same epoch should reuse the same profile: %q vs %q", first, second)
	}

	next, err := state.Current(start.Add(time.Minute))
	if err != nil {
		t.Fatalf("Current returned error: %v", err)
	}
	if next == first {
		t.Fatalf("new epoch should rotate profile: %q remained unchanged", next)
	}
}
