package planner

import (
	"fmt"
	"strings"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/traffic"
)

// Request is everything the planner needs before opening a connection.
type Request struct {
	Network       networkctx.Context
	Destination   string
	Protocol      string
	Purpose       string
	Carriers      []carrier.Candidate
	DPIStrategies []dpi.Strategy
}

// Plan is one combined routing decision.
//
// DPI target is not always the final website. When a tunnel carrier is used,
// the local network only sees the connection to that carrier endpoint, so DPI
// strategy learning must be attached to the endpoint instead.
type Plan struct {
	NetworkID    string
	TrafficClass traffic.Class
	Carrier      carrier.Decision
	DPI          dpi.Decision
	DPITarget    string
	Reason       string
}

// FailureScope tells the learner which layer actually failed.
// This prevents a DPI reset from poisoning a healthy physical route.
type FailureScope string

const (
	ScopeBoth    FailureScope = "both"
	ScopeCarrier FailureScope = "carrier"
	ScopeDPI     FailureScope = "dpi"
)

// Result feeds real measurements back into the relevant learners.
type Result struct {
	Plan                   Plan
	Destination            string
	Protocol               string
	Success                bool
	Scope                  FailureScope
	CarrierAlreadyObserved bool
	Latency                time.Duration
	Throughput             float64
	Failure                string
	At                     time.Time
}

// Planner joins route selection and DPI strategy selection.
type Planner struct {
	Carriers *carrier.Engine
	DPI      *dpi.Engine
}

// New creates a planner from the two persistent engines.
func New(carriers *carrier.Engine, dpiEngine *dpi.Engine) (*Planner, error) {
	if carriers == nil {
		return nil, fmt.Errorf("carrier engine is nil")
	}
	if dpiEngine == nil {
		return nil, fmt.Errorf("dpi engine is nil")
	}
	return &Planner{Carriers: carriers, DPI: dpiEngine}, nil
}

// Choose returns a complete plan for the request.
func (p *Planner) Choose(req Request) (Plan, error) {
	if p == nil || p.Carriers == nil || p.DPI == nil {
		return Plan{}, fmt.Errorf("planner is not initialized")
	}
	if strings.TrimSpace(req.Destination) == "" {
		return Plan{}, fmt.Errorf("destination must not be empty")
	}

	class := traffic.Classify(traffic.Hint{
		Destination: req.Destination,
		Protocol:    req.Protocol,
		Purpose:     req.Purpose,
	})

	carrierCtx := carrier.Context{
		NetworkID:    req.Network.ID,
		Destination:  req.Destination,
		TrafficClass: string(class),
		Protocol:     req.Protocol,
	}
	carrierDecision, err := p.Carriers.Choose(carrierCtx, req.Carriers)
	if err != nil {
		return Plan{}, fmt.Errorf("choose carrier: %w", err)
	}

	dpiTarget := req.Destination
	if carrierDecision.Carrier.Kind != carrier.KindDirect && carrierDecision.Carrier.Endpoint != "" {
		dpiTarget = carrierDecision.Carrier.Endpoint
	}

	dpiCtx := dpi.Context{
		NetworkID:    req.Network.ID,
		Destination:  dpiTarget,
		TrafficClass: string(class),
	}
	dpiStrategies := req.DPIStrategies
	if carrierDecision.Carrier.Kind == carrier.KindChameleonQUIC ||
		carrierDecision.Carrier.Kind == carrier.KindChameleonUDP {
		dpiStrategies = directOnlyDPI(req.DPIStrategies)
	}
	dpiDecision, err := p.DPI.Choose(dpiCtx, dpiStrategies)
	if err != nil {
		return Plan{}, fmt.Errorf("choose dpi strategy: %w", err)
	}

	return Plan{
		NetworkID:    req.Network.ID,
		TrafficClass: class,
		Carrier:      carrierDecision,
		DPI:          dpiDecision,
		DPITarget:    dpiTarget,
		Reason: fmt.Sprintf(
			"carrier=%s (%s); dpi=%s for %s (%s)",
			carrierDecision.Carrier.Name,
			carrierDecision.Reason,
			dpiDecision.Strategy.Name,
			dpiTarget,
			dpiDecision.Reason,
		),
	}, nil
}

// Observe records the result of executing a plan.
// One result updates carrier reachability and the DPI strategy used to reach
// the visible next hop.
func (p *Planner) Observe(result Result) error {
	if p == nil || p.Carriers == nil || p.DPI == nil {
		return fmt.Errorf("planner is not initialized")
	}
	at := result.At
	if at.IsZero() {
		at = time.Now()
	}

	scope := result.Scope
	if scope == "" {
		scope = ScopeBoth
	}

	carrierCtx := carrier.Context{
		NetworkID:    result.Plan.NetworkID,
		Destination:  result.Destination,
		TrafficClass: string(result.Plan.TrafficClass),
		Protocol:     result.Protocol,
	}
	dpiCtx := dpi.Context{
		NetworkID:    result.Plan.NetworkID,
		Destination:  result.Plan.DPITarget,
		TrafficClass: string(result.Plan.TrafficClass),
	}

	// Scope is respected for both success and failure. A successful TCP dial,
	// for example, proves the carrier but does not yet prove that TLS/HTTP
	// survived the path's DPI.
	if scope == ScopeCarrier || scope == ScopeBoth {
		if err := p.Carriers.Observe(carrier.Observation{
			Context:    carrierCtx,
			Carrier:    result.Plan.Carrier.Carrier.Name,
			Success:    result.Success,
			Latency:    result.Latency,
			Throughput: result.Throughput,
			Failure:    result.Failure,
			At:         at,
		}); err != nil {
			return fmt.Errorf("record carrier result: %w", err)
		}
	}

	if scope == ScopeDPI || scope == ScopeBoth {
		if err := p.DPI.Observe(dpi.Observation{
			Context:    dpiCtx,
			Strategy:   result.Plan.DPI.Strategy.Name,
			Success:    result.Success,
			Latency:    result.Latency,
			Throughput: result.Throughput,
			Failure:    result.Failure,
			At:         at,
		}); err != nil {
			return fmt.Errorf("record dpi result: %w", err)
		}
	}

	// If the route itself worked but DPI handling failed, record that the
	// carrier was reachable. This keeps a cheap direct route available while
	// the DPI engine explores split strategies.
	if !result.Success && scope == ScopeDPI && !result.CarrierAlreadyObserved {
		if err := p.Carriers.Observe(carrier.Observation{
			Context:    carrierCtx,
			Carrier:    result.Plan.Carrier.Carrier.Name,
			Success:    true,
			Latency:    result.Latency,
			Throughput: result.Throughput,
			At:         at,
		}); err != nil {
			return fmt.Errorf("record reachable carrier: %w", err)
		}
	}

	return nil
}


func directOnlyDPI(strategies []dpi.Strategy) []dpi.Strategy {
	for _, strategy := range strategies {
		if strategy.Name == "direct" {
			return []dpi.Strategy{strategy}
		}
	}
	defaults := dpi.DefaultStrategies()
	if len(defaults) == 0 {
		return nil
	}
	return defaults[:1]
}
