package autopilot

import (
	"testing"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/adaptive"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
)

func TestControllerLearnsWorkingCombination(t *testing.T) {
	paths, err := adaptive.NewPathLearner("")
	if err != nil {
		t.Fatal(err)
	}
	dpiEngine, err := dpi.NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(paths, dpiEngine)
	if err != nil {
		t.Fatal(err)
	}

	req := Request{
		NetworkID:   "mobile",
		Destination: "service.example",
		Traffic:     adaptive.TrafficWeb,
		Protocol:    dpi.ProtocolTLS,
		Carriers: []adaptive.PathCandidate{
			{Carrier: "quic", Endpoint: "edge", BaseCost: 1},
			{Carrier: "tcp-tls", Endpoint: "edge", BaseCost: 3},
		},
	}

	for i := 0; i < 4; i++ {
		err = c.Observe(Observation{
			Request: req,
			Candidate: adaptive.PathCandidate{
				Carrier: "tcp-tls", Endpoint: "edge",
				Strategy: string(dpi.StrategyStreamSplit), BaseCost: 5,
			},
			Success: true, Latency: 60 * time.Millisecond,
			Jitter: 4 * time.Millisecond, Throughput: 1_500_000,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	plan := c.Plan(req)
	if len(plan) == 0 {
		t.Fatal("empty plan")
	}
	if plan[0].Candidate.Carrier != "tcp-tls" ||
		plan[0].Candidate.Strategy != string(dpi.StrategyStreamSplit) {
		t.Fatalf("expected learned tcp-tls/stream-split first, got %+v", plan[0])
	}
}

func TestControllerKeepsPlanSmall(t *testing.T) {
	paths, _ := adaptive.NewPathLearner("")
	dpiEngine, _ := dpi.NewEngine("")
	c, _ := New(paths, dpiEngine)

	plan := c.Plan(Request{
		NetworkID: "n", Destination: "d",
		Traffic: adaptive.TrafficInteractive,
		Protocol: dpi.ProtocolTLS,
		Failure: dpi.FailureTLSHandshake,
		Carriers: []adaptive.PathCandidate{
			{Carrier: "quic", Endpoint: "e1"},
			{Carrier: "tcp-tls", Endpoint: "e2"},
		},
	})
	if len(plan) > 6 {
		t.Fatalf("probe plan is too large: %d", len(plan))
	}
}
