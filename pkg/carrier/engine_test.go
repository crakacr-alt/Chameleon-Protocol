package carrier

import (
	"path/filepath"
	"testing"
	"time"
)

func TestChooseDirectWithoutHistory(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{NetworkID: "wifi", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}
	decision, err := e.Choose(ctx, Defaults("server:9000", "server:443", ""))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Carrier.Name != "direct" {
		t.Fatalf("want direct, got %q", decision.Carrier.Name)
	}
}

func TestChooseFallbackAfterDirectFailure(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{NetworkID: "mobile", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}
	for i := 0; i < 2; i++ {
		if err := e.Observe(Observation{
			Context: ctx, Carrier: "direct", Success: false,
			Latency: 2 * time.Second, Failure: "timeout", At: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	decision, err := e.Choose(ctx, Defaults("server:9000", "server:443", ""))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Carrier.Name != "chameleon-udp" {
		t.Fatalf("want chameleon-udp, got %q", decision.Carrier.Name)
	}
}

func TestUDPDoesNotChooseTCPOnlyCarrier(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{NetworkID: "mobile", Destination: "game:5000", TrafficClass: "realtime", Protocol: "udp"}
	candidates := []Candidate{
		{Name: "tcp-only", Kind: KindChameleonTCP, SupportsTCP: true, Cost: 0},
		{Name: "udp", Kind: KindChameleonUDP, SupportsUDP: true, Cost: 1},
	}
	decision, err := e.Choose(ctx, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Carrier.Name != "udp" {
		t.Fatalf("want udp-compatible carrier, got %q", decision.Carrier.Name)
	}
}

func TestCarrierMemoryPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "carrier.json")
	ctx := Context{NetworkID: "wifi", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}

	e, err := NewEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Observe(Observation{Context: ctx, Carrier: "direct", Success: true, Latency: 10 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Snapshot(ctx)["direct"]
	if got.Attempts != 1 || got.Successes != 1 {
		t.Fatalf("unexpected persisted stats: %+v", got)
	}
}

func TestTLSCarrierPreferredOverRawTCPAfterDirectFailure(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{NetworkID: "mobile", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}
	if err := e.Observe(Observation{
		Context: ctx,
		Carrier: "direct",
		Success: false,
		Latency: time.Second,
		Failure: "blocked",
		At:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	candidates := WithTLS(Defaults("", "server:9443", ""), "server:443")
	decision, err := e.Choose(ctx, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Carrier.Name != "chameleon-tls" {
		t.Fatalf("want TLS fallback before raw TCP, got %q", decision.Carrier.Name)
	}
}

func TestStaleFailureDecaysBackTowardCheapDirect(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{NetworkID: "wifi", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}
	if err := e.Observe(Observation{
		Context: ctx,
		Carrier: "direct",
		Success: false,
		Latency: 2 * time.Second,
		Failure: "old timeout",
		At:      time.Now().Add(-14 * 24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	candidates := WithQUIC(Defaults("", "server:9443", ""), "server:443")
	decision, err := e.Choose(ctx, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Carrier.Name != "direct" {
		t.Fatalf("stale evidence should decay toward direct, got %q", decision.Carrier.Name)
	}
}

func TestHasEvidenceIsContextSpecific(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	candidates := WithQUIC(Defaults("", "", ""), "server:443")
	known := Context{NetworkID: "wifi", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}
	unknown := Context{NetworkID: "mobile", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}

	if err := e.Observe(Observation{
		Context: known,
		Carrier: "direct",
		Success: true,
		At:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if !e.HasEvidence(known, candidates) {
		t.Fatal("expected evidence for known context")
	}
	if e.HasEvidence(unknown, candidates) {
		t.Fatal("evidence must not leak across network contexts")
	}
}

func TestQUICPreferredBeforeTLSAfterDirectFailure(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{NetworkID: "mobile", Destination: "example.com:443", TrafficClass: "web", Protocol: "tcp"}
	if err := e.Observe(Observation{
		Context: ctx,
		Carrier: "direct",
		Success: false,
		Latency: time.Second,
		Failure: "blocked",
		At:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	candidates := WithQUIC(WithTLS(Defaults("", "server:9443", ""), "server:443"), "server:443")
	decision, err := e.Choose(ctx, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Carrier.Name != "chameleon-quic" {
		t.Fatalf("want QUIC fallback before TLS, got %q", decision.Carrier.Name)
	}
}


func TestCarrierTracksLatencyJitter(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{NetworkID: "wifi", Destination: "game.example:443", TrafficClass: "realtime", Protocol: "tcp"}
	for _, latency := range []time.Duration{20 * time.Millisecond, 50 * time.Millisecond} {
		if err := e.Observe(Observation{
			Context: ctx,
			Carrier: "direct",
			Success: true,
			Latency: latency,
			At:      time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	stats := e.Snapshot(ctx)["direct"]
	if stats.LastLatency != 50*time.Millisecond {
		t.Fatalf("unexpected last latency %s", stats.LastLatency)
	}
	if stats.AvgJitter <= 0 {
		t.Fatalf("expected non-zero jitter, got %s", stats.AvgJitter)
	}
}
