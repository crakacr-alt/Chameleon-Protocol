package probe

import (
	"errors"
	"testing"
	"time"
)

func TestSummarizeMetrics(t *testing.T) {
	summary := Summarize([]Sample{
		{Latency: 10 * time.Millisecond},
		{Latency: 14 * time.Millisecond},
		{Err: errors.New("timeout")},
		{Latency: 12 * time.Millisecond},
	})
	if summary.Attempts != 4 || summary.Successes != 3 || summary.Failures != 1 {
		t.Fatalf("unexpected counts %+v", summary)
	}
	if summary.AvgLatency != 12*time.Millisecond {
		t.Fatalf("unexpected average latency %s", summary.AvgLatency)
	}
	// Successful latencies are 10,14,12ms => deltas 4 and 2 => jitter 3ms.
	if summary.Jitter != 3*time.Millisecond {
		t.Fatalf("unexpected jitter %s", summary.Jitter)
	}
	if summary.LossRate != 0.25 {
		t.Fatalf("unexpected loss rate %f", summary.LossRate)
	}
}
