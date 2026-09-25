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
