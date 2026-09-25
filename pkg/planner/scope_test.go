package planner

import (
	"testing"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
)

func TestCarrierOnlySuccessDoesNotMarkDPISuccess(t *testing.T) {
	p := newTestPlanner(t)
	req := Request{
		Network:       networkctx.Context{ID: "scope-test"},
		Destination:   "example.com:443",
		Protocol:      "tcp",
		Purpose:       "web",
		Carriers:      carrier.Defaults("server:9000", "server:443", ""),
		DPIStrategies: dpi.DefaultStrategies(),
	}
	plan, err := p.Choose(req)
	if err != nil {
		t.Fatal(err)
	}

	if err := p.Observe(Result{
		Plan:        plan,
		Destination: req.Destination,
		Protocol:    req.Protocol,
		Success:     true,
		Scope:       ScopeCarrier,
		Latency:     20 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}

	carrierStats := p.Carriers.Snapshot(carrier.Context{
		NetworkID:    req.Network.ID,
		Destination:  req.Destination,
		TrafficClass: string(plan.TrafficClass),
		Protocol:     req.Protocol,
	})
	if carrierStats["direct"].Successes != 1 {
		t.Fatalf("carrier success was not recorded: %+v", carrierStats)
	}

	dpiStats := p.DPI.Snapshot(dpi.Context{
		NetworkID:    req.Network.ID,
		Destination:  req.Destination,
		TrafficClass: string(plan.TrafficClass),
	})
	if len(dpiStats) != 0 {
		t.Fatalf("DPI must remain unproven after carrier-only success: %+v", dpiStats)
	}
}


func TestDPIFailureCanSkipDuplicateCarrierCredit(t *testing.T) {
	p := newTestPlanner(t)
	req := Request{
		Network:       networkctx.Context{ID: "scope-dedup"},
		Destination:   "example.com:443",
		Protocol:      "tcp",
		Purpose:       "web",
		Carriers:      carrier.Defaults("", "server:443", ""),
		DPIStrategies: dpi.DefaultStrategies(),
	}
	plan, err := p.Choose(req)
	if err != nil {
		t.Fatal(err)
	}

	if err := p.Observe(Result{
		Plan:        plan,
		Destination: req.Destination,
		Protocol:    req.Protocol,
		Success:     true,
		Scope:       ScopeCarrier,
		Latency:     20 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.Observe(Result{
		Plan:                   plan,
		Destination:            req.Destination,
		Protocol:               req.Protocol,
		Success:                false,
		Scope:                  ScopeDPI,
		CarrierAlreadyObserved: true,
		Latency:                30 * time.Millisecond,
		Failure:                "early EOF",
	}); err != nil {
		t.Fatal(err)
	}

	carrierStats := p.Carriers.Snapshot(carrier.Context{
		NetworkID:    req.Network.ID,
		Destination:  req.Destination,
		TrafficClass: string(plan.TrafficClass),
		Protocol:     req.Protocol,
	})["direct"]
	if carrierStats.Successes != 1 || carrierStats.Attempts != 1 {
		t.Fatalf("carrier should be credited once, got %+v", carrierStats)
	}

	dpiStats := p.DPI.Snapshot(dpi.Context{
		NetworkID:    req.Network.ID,
		Destination:  req.Destination,
		TrafficClass: string(plan.TrafficClass),
	})["direct"]
	if dpiStats.Failures != 1 {
		t.Fatalf("DPI failure was not recorded: %+v", dpiStats)
	}
}
