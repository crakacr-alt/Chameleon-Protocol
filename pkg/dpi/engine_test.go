package dpi

import (
	"path/filepath"
	"testing"
	"time"
)

func TestChooseStartsWithDirect(t *testing.T) {
	engine, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}

	decision, err := engine.Choose(Context{
		NetworkID:    "wifi-home",
		Destination:  "example.com:443",
		TrafficClass: "web",
	}, DefaultStrategies())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Strategy.Name != "direct" {
		t.Fatalf("expected direct first, got %q", decision.Strategy.Name)
	}
	if decision.Known {
		t.Fatal("new context must not be marked as learned")
	}
}

func TestChooseEscalatesAfterDirectFailures(t *testing.T) {
	engine, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}

	ctx := Context{NetworkID: "mobile-a", Destination: "video.example:443", TrafficClass: "streaming"}
	now := time.Now()

	for i := 0; i < 2; i++ {
		if err := engine.Observe(Observation{
			Context:  ctx,
			Strategy: "direct",
			Success:  false,
			Latency:  2 * time.Second,
			Failure:  "timeout",
			At:       now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	decision, err := engine.Choose(ctx, DefaultStrategies())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Strategy.Name != "split-early" {
		t.Fatalf("expected split-early after direct failures, got %q", decision.Strategy.Name)
	}
}

func TestChooseLearnsSuccessfulStrategy(t *testing.T) {
	engine, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}

	ctx := Context{NetworkID: "mobile-a", Destination: "video.example:443", TrafficClass: "streaming"}
	if err := engine.Observe(Observation{
		Context:    ctx,
		Strategy:   "direct",
		Success:    false,
		Latency:    time.Second,
		Throughput: 0,
		At:         time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := engine.Observe(Observation{
			Context:    ctx,
			Strategy:   "split-early",
			Success:    true,
			Latency:    50 * time.Millisecond,
			Throughput: 3 * 1024 * 1024,
			At:         time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	decision, err := engine.Choose(ctx, DefaultStrategies())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Strategy.Name != "split-early" {
		t.Fatalf("expected learned split-early, got %q", decision.Strategy.Name)
	}
	if !decision.Known {
		t.Fatal("learned decision must be marked known")
	}
}

func TestContextsDoNotLeakIntoEachOther(t *testing.T) {
	engine, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}

	mobile := Context{NetworkID: "mobile", Destination: "site.example:443", TrafficClass: "web"}
	wifi := Context{NetworkID: "wifi", Destination: "site.example:443", TrafficClass: "web"}

	if err := engine.Observe(Observation{
		Context:  mobile,
		Strategy: "direct",
		Success:  false,
		At:       time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	decision, err := engine.Choose(wifi, DefaultStrategies())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Strategy.Name != "direct" {
		t.Fatalf("mobile history leaked into wifi context: got %q", decision.Strategy.Name)
	}
}

func TestEnginePersistsMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dpi-memory.json")
	ctx := Context{NetworkID: "wifi", Destination: "example.org:443", TrafficClass: "web"}

	engine, err := NewEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Observe(Observation{
		Context:    ctx,
		Strategy:   "split-early",
		Success:    true,
		Latency:    20 * time.Millisecond,
		Throughput: 1024,
		At:         time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Snapshot(ctx)["split-early"]
	if got.Attempts != 1 || got.Successes != 1 {
		t.Fatalf("unexpected persisted stats: %+v", got)
	}
}
