package morph

import (
	"testing"
	"time"
)

func TestJitterConfigNextWithinBounds(t *testing.T) {
	t.Parallel()

	cfg := JitterConfig{
		BaseDelay: 2 * time.Millisecond,
		MinJitter: 1 * time.Millisecond,
		MaxJitter: 3 * time.Millisecond,
	}

	delay, err := cfg.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}

	if delay < 3*time.Millisecond || delay > 5*time.Millisecond {
		t.Fatalf("unexpected delay %v; want range [%v, %v]", delay, 3*time.Millisecond, 5*time.Millisecond)
	}
}
