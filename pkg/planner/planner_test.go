package planner

import (
	"testing"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
)

func newTestPlanner(t *testing.T) *Planner {
	t.Helper()
	carrierEngine, err := carrier.NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	dpiEngine, err := dpi.NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	p, err := New(carrierEngine, dpiEngine)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlannerStartsDirect(t *testing.T) {
	p := newTestPlanner(t)
	req := Request{
		Network:       networkctx.Context{ID: "wifi-home"},
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
	if plan.Carrier.Carrier.Name != "direct" {
		t.Fatalf("want direct carrier, got %q", plan.Carrier.Carrier.Name)
	}
	if plan.DPI.Strategy.Name != "direct" {
		t.Fatalf("want direct dpi, got %q", plan.DPI.Strategy.Name)
	}
	if plan.DPITarget != req.Destination {
		t.Fatalf("want destination as dpi target, got %q", plan.DPITarget)
	}
}

func TestPlannerLearnsCarrierAndRetargetsDPI(t *testing.T) {
	p := newTestPlanner(t)
	req := Request{
		Network:       networkctx.Context{ID: "mobile-a"},
		Destination:   "example.com:443",
		Protocol:      "tcp",
		Purpose:       "web",
		Carriers:      carrier.Defaults("server:9000", "server:443", ""),
		DPIStrategies: dpi.DefaultStrategies(),
	}

	first, err := p.Choose(req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := p.Observe(Result{
			Plan: first, Destination: req.Destination, Protocol: req.Protocol,
			Success: false, Latency: 2 * time.Second, Failure: "timeout",
		}); err != nil {
			t.Fatal(err)
		}
	}

	second, err := p.Choose(req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Carrier.Carrier.Name != "chameleon-udp" {
		t.Fatalf("want chameleon-udp fallback, got %q", second.Carrier.Carrier.Name)
	}
	if second.DPITarget != "server:9000" {
		t.Fatalf("want carrier endpoint as dpi target, got %q", second.DPITarget)
	}
}

func TestPlannerKeepsNetworkLearningSeparate(t *testing.T) {
	p := newTestPlanner(t)
	base := Request{
		Destination:   "example.com:443",
		Protocol:      "tcp",
		Purpose:       "web",
		Carriers:      carrier.Defaults("server:9000", "server:443", ""),
		DPIStrategies: dpi.DefaultStrategies(),
	}
	base.Network = networkctx.Context{ID: "mobile"}

	plan, err := p.Choose(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Observe(Result{
		Plan: plan, Destination: base.Destination, Protocol: base.Protocol,
		Success: false, Latency: time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	base.Network = networkctx.Context{ID: "wifi"}
	wifiPlan, err := p.Choose(base)
	if err != nil {
		t.Fatal(err)
	}
	if wifiPlan.Carrier.Carrier.Name != "direct" || wifiPlan.DPI.Strategy.Name != "direct" {
		t.Fatalf("mobile history leaked into wifi plan: %+v", wifiPlan)
	}
}

func TestDPIFailureKeepsDirectCarrierAndEscalatesStrategy(t *testing.T) {
	p := newTestPlanner(t)
	req := Request{
		Network:       networkctx.Context{ID: "mobile-dpi"},
		Destination:   "example.com:443",
		Protocol:      "tcp",
		Purpose:       "web",
		Carriers:      carrier.Defaults("server:9000", "server:443", ""),
		DPIStrategies: dpi.DefaultStrategies(),
	}

	first, err := p.Choose(req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := p.Observe(Result{
			Plan:        first,
			Destination: req.Destination,
			Protocol:    req.Protocol,
			Success:     false,
			Scope:       ScopeDPI,
			Latency:     40 * time.Millisecond,
			Failure:     "connection reset after transport established",
		}); err != nil {
			t.Fatal(err)
		}
	}

	next, err := p.Choose(req)
	if err != nil {
		t.Fatal(err)
	}
	if next.Carrier.Carrier.Name != "direct" {
		t.Fatalf("DPI failure must not poison direct carrier, got %q", next.Carrier.Carrier.Name)
	}
	if next.DPI.Strategy.Name != "split-early" {
		t.Fatalf("expected DPI escalation to split-early, got %q", next.DPI.Strategy.Name)
	}
}


func TestPlannerUsesDirectDPIForQUIC(t *testing.T) {
	p := newTestPlanner(t)
	networkID := "mobile-quic"
	endpoint := "server:443"

	// Even if split-early previously worked for this endpoint, it is a TCP
	// first-write technique and must not be attached to the QUIC carrier.
	if err := p.DPI.Observe(dpi.Observation{
		Context: dpi.Context{
			NetworkID:    networkID,
			Destination:  endpoint,
			TrafficClass: "web",
		},
		Strategy: "split-early",
		Success:  true,
		At:       time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	// Make direct carrier unattractive so the planner selects QUIC.
	carrierCtx := carrier.Context{
		NetworkID:    networkID,
		Destination:  "example.com:443",
		TrafficClass: "web",
		Protocol:     "tcp",
	}
	if err := p.Carriers.Observe(carrier.Observation{
		Context: carrierCtx,
		Carrier: "direct",
		Success: false,
		At:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	req := Request{
		Network:       networkctx.Context{ID: networkID},
		Destination:   "example.com:443",
		Protocol:      "tcp",
		Purpose:       "web",
		Carriers:      carrier.WithQUIC(carrier.Defaults("", "", ""), endpoint),
		DPIStrategies: dpi.DefaultStrategies(),
	}
	plan, err := p.Choose(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Carrier.Carrier.Kind != carrier.KindChameleonQUIC {
		t.Fatalf("want QUIC carrier, got %s", plan.Carrier.Carrier.Kind)
	}
	if plan.DPI.Strategy.Name != "direct" {
		t.Fatalf("QUIC must use direct DPI policy, got %q", plan.DPI.Strategy.Name)
	}
}
