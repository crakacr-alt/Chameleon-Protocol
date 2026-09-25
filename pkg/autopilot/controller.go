// Package autopilot combines route learning with the DPI policy engine.
package autopilot

import (
	"fmt"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/adaptive"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
)

// Request is the context for one connection attempt.
type Request struct {
	NetworkID   string
	Destination string
	Traffic     adaptive.TrafficClass
	Protocol    dpi.ProtocolClass
	Failure     dpi.FailureKind
	Carriers    []adaptive.PathCandidate
}

// Choice is one fully-scored carrier + strategy pair.
type Choice struct {
	Candidate adaptive.PathCandidate
	Score     float64
	Reason    string
}

// Observation feeds real connection results back into both learners.
type Observation struct {
	Request    Request
	Candidate  adaptive.PathCandidate
	Success    bool
	Failure    dpi.FailureKind
	Latency    time.Duration
	Jitter     time.Duration
	Loss       float64
	Throughput float64
	Overhead   float64
	At         time.Time
}

// Controller is the low-cost self-learning control plane.
// It owns no sockets and therefore can be reused by Linux, Windows, Android,
// router and embedded frontends without competing for a VPN/TUN interface.
type Controller struct {
	Paths *adaptive.PathLearner
	DPI   *dpi.Engine
}

// New creates an autopilot from the two persistent learners.
func New(paths *adaptive.PathLearner, dpiEngine *dpi.Engine) (*Controller, error) {
	if paths == nil {
		return nil, fmt.Errorf("path learner is required")
	}
	if dpiEngine == nil {
		return nil, fmt.Errorf("dpi engine is required")
	}
	return &Controller{Paths: paths, DPI: dpiEngine}, nil
}

// Plan produces a short ordered list. It intentionally avoids trying every
// strategy on every carrier: only the first three DPI policies are crossed with
// the available carriers, keeping startup probes cheap.
func (c *Controller) Plan(req Request) []Choice {
	if c == nil || c.Paths == nil || c.DPI == nil || len(req.Carriers) == 0 {
		return nil
	}

	strategies := c.DPI.Plan(req.NetworkID, req.Destination, req.Protocol, req.Failure)
	if len(strategies) > 3 {
		strategies = strategies[:3]
	}
	if len(strategies) == 0 {
		strategies = []dpi.RankedStrategy{{Spec: dpi.StrategySpec{Name: dpi.StrategyDirect}}}
	}

	candidates := make([]adaptive.PathCandidate, 0, len(req.Carriers)*len(strategies))
	for _, carrier := range req.Carriers {
		for _, strategy := range strategies {
			candidate := carrier
			candidate.Strategy = string(strategy.Spec.Name)
			// Strategy cost is folded into BaseCost so route ranking naturally
			// prefers the least invasive solution when success is comparable.
			candidate.BaseCost += strategy.Spec.Cost
			candidates = append(candidates, candidate)
		}
	}

	ranked := c.Paths.Rank(req.NetworkID, req.Destination, req.Traffic, candidates)
	out := make([]Choice, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, Choice{
			Candidate: item.Candidate,
			Score:     item.Score,
			Reason:    item.Reason,
		})
	}
	return out
}

// Observe learns from a completed attempt. One result updates route quality and
// the DPI cache, so the next connection on the same network can start closer to
// the last known-good combination.
func (c *Controller) Observe(obs Observation) error {
	if c == nil || c.Paths == nil || c.DPI == nil {
		return fmt.Errorf("autopilot is not initialized")
	}
	at := obs.At
	if at.IsZero() {
		at = time.Now()
	}
	if err := c.Paths.Observe(adaptive.PathObservation{
		NetworkID:   obs.Request.NetworkID,
		Destination: obs.Request.Destination,
		Traffic:     obs.Request.Traffic,
		Candidate:   obs.Candidate,
		Success:     obs.Success,
		Latency:     obs.Latency,
		Jitter:      obs.Jitter,
		Loss:        obs.Loss,
		Throughput:  obs.Throughput,
		Overhead:    obs.Overhead,
		At:          at,
	}); err != nil {
		return err
	}

	return c.DPI.Observe(dpi.Result{
		NetworkID:   obs.Request.NetworkID,
		Destination: obs.Request.Destination,
		Protocol:    obs.Request.Protocol,
		Strategy:    dpi.Strategy(obs.Candidate.Strategy),
		Failure:     obs.Failure,
		Success:     obs.Success,
		Latency:     obs.Latency,
		At:          at,
	})
}
