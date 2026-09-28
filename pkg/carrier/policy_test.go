package carrier

import (
	"testing"
	"time"
)

func TestScorePolicyChangesRouteChoice(t *testing.T) {
	engine, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{
		NetworkID:    "desktop-test",
		Destination:  "media.example:443",
		TrafficClass: "web",
		Protocol:     "tcp",
	}
	candidates := []Candidate{
		{Name: "low-latency", Kind: KindChameleonQUIC, Cost: 0.4, SupportsTCP: true},
		{Name: "high-throughput", Kind: KindChameleonTLS, Cost: 0.4, SupportsTCP: true},
	}

	if err := engine.Observe(Observation{
		Context: ctx, Carrier: "low-latency", Success: true,
		Latency: 20 * time.Millisecond, Throughput: 1 * 1024 * 1024,
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Observe(Observation{
		Context: ctx, Carrier: "high-throughput", Success: true,
		Latency: time.Second, Throughput: 20 * 1024 * 1024,
	}); err != nil {
		t.Fatal(err)
	}

	engine.SetScorePolicy(ScorePolicy{
		LatencyScale: 2.2, JitterScale: 2.4,
		ThroughputScale: 0.45, CostScale: 0.75,
	})
	gaming, err := engine.Choose(ctx, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if gaming.Carrier.Name != "low-latency" {
		t.Fatalf("gaming policy chose %q", gaming.Carrier.Name)
	}

	engine.SetScorePolicy(ScorePolicy{
		LatencyScale: 0.55, JitterScale: 0.8,
		ThroughputScale: 2.25, CostScale: 0.75,
	})
	streaming, err := engine.Choose(ctx, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if streaming.Carrier.Name != "high-throughput" {
		t.Fatalf("streaming policy chose %q", streaming.Carrier.Name)
	}
}
