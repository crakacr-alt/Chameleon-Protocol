// Package dpi contains a policy layer for adaptive DPI workarounds.
//
// It does not intercept packets itself. Platform adapters implement the actual
// transport manipulation, while this package decides when a strategy is worth
// trying and remembers the least intrusive strategy that worked.
package dpi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Strategy names one transport manipulation policy.
type Strategy string

const (
	StrategyDirect         Strategy = "direct"
	StrategyStreamSplit    Strategy = "stream-split"
	StrategyTLSRecordSplit Strategy = "tls-record-split"
	StrategyDisorder       Strategy = "disorder"
	StrategyFakePacket     Strategy = "fake-packet"
	StrategyUDPShape       Strategy = "udp-shape"
)

// FailureKind is a high-level symptom. The detector is intentionally separate
// from the policy so different operating systems can supply different signals.
type FailureKind string

const (
	FailureNone          FailureKind = "none"
	FailureTimeout       FailureKind = "timeout"
	FailureReset         FailureKind = "reset"
	FailureTLSHandshake  FailureKind = "tls-handshake"
	FailureRedirect      FailureKind = "redirect"
	FailureUDPBlackhole  FailureKind = "udp-blackhole"
	FailureHighLoss      FailureKind = "high-loss"
	FailureIncompatible  FailureKind = "incompatible"
)

// ProtocolClass keeps policies scoped to the protocol they can actually help.
type ProtocolClass string

const (
	ProtocolAny   ProtocolClass = "any"
	ProtocolHTTP  ProtocolClass = "http"
	ProtocolTLS   ProtocolClass = "tls"
	ProtocolQUIC  ProtocolClass = "quic"
	ProtocolUDP   ProtocolClass = "udp"
)

// StrategySpec describes one policy and its expected cost.
// Lower Cost means less latency/overhead and is preferred when success is equal.
type StrategySpec struct {
	Name      Strategy
	Protocols []ProtocolClass
	Cost      float64
}

// Result is one observed attempt.
type Result struct {
	NetworkID   string
	Destination string
	Protocol    ProtocolClass
	Strategy    Strategy
	Failure     FailureKind
	Success     bool
	Latency     time.Duration
	At          time.Time
}

// Memory is the compact score for one strategy in one context.
type Memory struct {
	Attempts            int
	Successes           int
	Failures            int
	ConsecutiveFailures int
	LatencyEWMA         time.Duration
	LastUsed            time.Time
	LastSuccess         time.Time
}

// RankedStrategy is returned by Plan.
type RankedStrategy struct {
	Spec   StrategySpec
	Score  float64
	Known  bool
	Reason string
}

// Engine implements a ByeDPI-style automatic retry/cache concept generalized
// into a reusable policy layer. Direct traffic is always tried first unless the
// exact context already has strong evidence that another strategy is better.
type Engine struct {
	mu        sync.Mutex
	StorePath string
	Alpha     float64
	Memory    map[string]Memory
	Specs     []StrategySpec
}

// DefaultSpecs are ordered from least to most intrusive.
func DefaultSpecs() []StrategySpec {
	return []StrategySpec{
		{Name: StrategyDirect, Protocols: []ProtocolClass{ProtocolAny}, Cost: 0},
		{Name: StrategyStreamSplit, Protocols: []ProtocolClass{ProtocolHTTP, ProtocolTLS}, Cost: 2},
		{Name: StrategyTLSRecordSplit, Protocols: []ProtocolClass{ProtocolTLS}, Cost: 3},
		{Name: StrategyDisorder, Protocols: []ProtocolClass{ProtocolTLS, ProtocolHTTP}, Cost: 6},
		{Name: StrategyFakePacket, Protocols: []ProtocolClass{ProtocolTLS, ProtocolHTTP}, Cost: 9},
		{Name: StrategyUDPShape, Protocols: []ProtocolClass{ProtocolQUIC, ProtocolUDP}, Cost: 4},
	}
}

// NewEngine opens persistent strategy memory. Empty path means memory-only.
func NewEngine(storePath string) (*Engine, error) {
	e := &Engine{
		StorePath: storePath,
		Alpha:     0.30,
		Memory:    make(map[string]Memory),
		Specs:     DefaultSpecs(),
	}
	if storePath == "" {
		return e, nil
	}
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return nil, fmt.Errorf("create dpi store directory: %w", err)
	}
	if data, err := os.ReadFile(storePath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, e); err != nil {
			return nil, fmt.Errorf("load dpi memory: %w", err)
		}
		e.StorePath = storePath
		if e.Alpha <= 0 || e.Alpha > 1 {
			e.Alpha = 0.30
		}
		if e.Memory == nil {
			e.Memory = make(map[string]Memory)
		}
		if len(e.Specs) == 0 {
			e.Specs = DefaultSpecs()
		}
	}
	return e, nil
}

// Observe records an attempt. A successful strategy becomes cheap to reuse in
// exactly the same network/destination/protocol context.
func (e *Engine) Observe(r Result) error {
	if e == nil {
		return nil
	}
	if strings.TrimSpace(r.Destination) == "" || r.Strategy == "" {
		return fmt.Errorf("destination and strategy must not be empty")
	}
	if r.NetworkID == "" {
		r.NetworkID = "default"
	}
	if r.Protocol == "" {
		r.Protocol = ProtocolAny
	}
	if r.At.IsZero() {
		r.At = time.Now()
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	k := memoryKey(r.NetworkID, r.Destination, r.Protocol, r.Strategy)
	m := e.Memory[k]
	m.Attempts++
	if r.Success {
		m.Successes++
		m.ConsecutiveFailures = 0
		m.LastSuccess = r.At
	} else {
		m.Failures++
		m.ConsecutiveFailures++
	}
	if r.Latency > 0 {
		if m.Attempts <= 1 || m.LatencyEWMA == 0 {
			m.LatencyEWMA = r.Latency
		} else {
			m.LatencyEWMA = time.Duration(float64(m.LatencyEWMA)*(1-e.Alpha) + float64(r.Latency)*e.Alpha)
		}
	}
	m.LastUsed = r.At
	e.Memory[k] = m
	return e.saveLocked()
}

// Plan returns applicable strategies from most promising to least promising.
// A failure hint raises strategies that are relevant to the observed symptom.
func (e *Engine) Plan(networkID, destination string, protocol ProtocolClass, failure FailureKind) []RankedStrategy {
	if e == nil {
		return nil
	}
	if networkID == "" {
		networkID = "default"
	}
	if protocol == "" {
		protocol = ProtocolAny
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	var out []RankedStrategy
	for _, spec := range e.Specs {
		if !supports(spec, protocol) {
			continue
		}
		m, known := e.Memory[memoryKey(networkID, destination, protocol, spec.Name)]
		score := 30 - spec.Cost
		reason := "not tried in this context"
		if known && m.Attempts > 0 {
			rate := float64(m.Successes) / float64(m.Attempts)
			score += rate*55 - float64(m.ConsecutiveFailures)*18
			if m.LatencyEWMA > 0 {
				score -= minFloat(float64(m.LatencyEWMA)/float64(100*time.Millisecond), 10)
			}
			reason = fmt.Sprintf("%d attempts, %.0f%% success, failures=%d",
				m.Attempts, rate*100, m.ConsecutiveFailures)
		}
		score += failureAffinity(spec.Name, failure)
		if spec.Name == StrategyDirect && failure != FailureNone {
			score -= 25
		}
		out = append(out, RankedStrategy{Spec: spec, Score: score, Known: known, Reason: reason})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Spec.Cost < out[j].Spec.Cost
		}
		return out[i].Score > out[j].Score
	})
	return out
}

// Best returns the first strategy from Plan, or direct if no specific strategy applies.
func (e *Engine) Best(networkID, destination string, protocol ProtocolClass, failure FailureKind) Strategy {
	plan := e.Plan(networkID, destination, protocol, failure)
	if len(plan) == 0 {
		return StrategyDirect
	}
	return plan[0].Spec.Name
}

func (e *Engine) saveLocked() error {
	if e.StorePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dpi memory: %w", err)
	}
	tmp := e.StorePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write dpi memory: %w", err)
	}
	if err := os.Rename(tmp, e.StorePath); err != nil {
		return fmt.Errorf("replace dpi memory: %w", err)
	}
	return nil
}

func supports(spec StrategySpec, protocol ProtocolClass) bool {
	for _, p := range spec.Protocols {
		if p == ProtocolAny || p == protocol {
			return true
		}
	}
	return false
}

func failureAffinity(strategy Strategy, failure FailureKind) float64 {
	switch failure {
	case FailureTLSHandshake:
		switch strategy {
		case StrategyTLSRecordSplit:
			return 18
		case StrategyStreamSplit:
			return 12
		case StrategyDisorder:
			return 8
		}
	case FailureTimeout, FailureReset:
		switch strategy {
		case StrategyStreamSplit:
			return 12
		case StrategyTLSRecordSplit:
			return 10
		case StrategyDisorder:
			return 8
		}
	case FailureUDPBlackhole, FailureHighLoss:
		if strategy == StrategyUDPShape {
			return 16
		}
	case FailureIncompatible:
		if strategy == StrategyDirect {
			return 20
		}
	}
	return 0
}

func memoryKey(networkID, destination string, protocol ProtocolClass, strategy Strategy) string {
	return strings.Join([]string{
		clean(networkID), clean(destination), clean(string(protocol)), clean(string(strategy)),
	}, "|")
}

func clean(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
