package adaptive

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPathLearnerRanksSuccessfulLowLatencyPath(t *testing.T) {
	l, err := NewPathLearner(filepath.Join(t.TempDir(), "paths.json"))
	if err != nil {
		t.Fatal(err)
	}

	fast := PathCandidate{Carrier: "quic", Endpoint: "edge", Strategy: "direct", BaseCost: 1}
	slow := PathCandidate{Carrier: "tcp-tls", Endpoint: "edge", Strategy: "split", BaseCost: 4}

	for i := 0; i < 6; i++ {
		if err := l.Observe(PathObservation{
			NetworkID: "mobile-a", Destination: "example.com", Traffic: TrafficInteractive,
			Candidate: fast, Success: true, Latency: 35 * time.Millisecond,
			Jitter: 4 * time.Millisecond, Loss: 0.01, Throughput: 2_000_000,
		}); err != nil {
			t.Fatal(err)
		}
		if err := l.Observe(PathObservation{
			NetworkID: "mobile-a", Destination: "example.com", Traffic: TrafficInteractive,
			Candidate: slow, Success: true, Latency: 180 * time.Millisecond,
			Jitter: 30 * time.Millisecond, Loss: 0.02, Throughput: 1_000_000, Overhead: 0.08,
		}); err != nil {
			t.Fatal(err)
		}
	}

	ranked := l.Rank("mobile-a", "example.com", TrafficInteractive, []PathCandidate{slow, fast})
	if len(ranked) != 2 {
		t.Fatalf("ranked %d candidates", len(ranked))
	}
	if ranked[0].Candidate.Carrier != "quic" {
		t.Fatalf("expected quic first, got %s (%v)", ranked[0].Candidate.Carrier, ranked)
	}
}

func TestPathLearnerSeparatesNetworks(t *testing.T) {
	l, err := NewPathLearner("")
	if err != nil {
		t.Fatal(err)
	}
	quic := PathCandidate{Carrier: "quic", Endpoint: "edge", Strategy: "direct"}
	tcp := PathCandidate{Carrier: "tcp-tls", Endpoint: "edge", Strategy: "direct"}

	for i := 0; i < 4; i++ {
		_ = l.Observe(PathObservation{
			NetworkID: "wifi", Destination: "service", Traffic: TrafficWeb,
			Candidate: quic, Success: true, Latency: 20 * time.Millisecond,
		})
		_ = l.Observe(PathObservation{
			NetworkID: "mobile", Destination: "service", Traffic: TrafficWeb,
			Candidate: quic, Success: false, Latency: 2 * time.Second, Loss: 1,
		})
		_ = l.Observe(PathObservation{
			NetworkID: "mobile", Destination: "service", Traffic: TrafficWeb,
			Candidate: tcp, Success: true, Latency: 70 * time.Millisecond,
		})
	}

	wifi := l.Rank("wifi", "service", TrafficWeb, []PathCandidate{tcp, quic})
	if wifi[0].Candidate.Carrier != "quic" {
		t.Fatalf("wifi should prefer quic: %#v", wifi)
	}
	mobile := l.Rank("mobile", "service", TrafficWeb, []PathCandidate{quic, tcp})
	if mobile[0].Candidate.Carrier != "tcp-tls" {
		t.Fatalf("mobile should prefer tcp-tls: %#v", mobile)
	}
}

func TestPathLearnerSwitchesAfterRepeatedFailures(t *testing.T) {
	l, err := NewPathLearner("")
	if err != nil {
		t.Fatal(err)
	}
	current := PathCandidate{Carrier: "quic", Endpoint: "edge", Strategy: "direct"}
	next := PathCandidate{Carrier: "tcp-tls", Endpoint: "edge", Strategy: "direct"}

	for i := 0; i < 3; i++ {
		_ = l.Observe(PathObservation{
			NetworkID: "lte", Destination: "service", Traffic: TrafficWeb,
			Candidate: current, Success: false, Loss: 1, Latency: time.Second,
		})
	}
	if !l.ShouldSwitch("lte", "service", TrafficWeb, current, next, 1000) {
		t.Fatal("expected repeated failures to force a switch")
	}
}

func TestPathLearnerPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paths.json")
	l, err := NewPathLearner(path)
	if err != nil {
		t.Fatal(err)
	}
	c := PathCandidate{Carrier: "relay", Endpoint: "r1", Strategy: "direct"}
	if err := l.Observe(PathObservation{
		NetworkID: "n1", Destination: "d1", Traffic: TrafficStreaming,
		Candidate: c, Success: true, Latency: 50 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewPathLearner(path)
	if err != nil {
		t.Fatal(err)
	}
	stats, ok := reloaded.Stats("n1", "d1", TrafficStreaming, c)
	if !ok || stats.Successes != 1 {
		t.Fatalf("persistence failed: ok=%v stats=%+v", ok, stats)
	}
}
