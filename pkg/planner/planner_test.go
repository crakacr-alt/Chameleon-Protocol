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
