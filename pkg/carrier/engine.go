package carrier

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kind describes how traffic reaches its next hop.
type Kind string

const (
	KindDirect       Kind = "direct"
	KindChameleonUDP  Kind = "chameleon-udp"
	KindChameleonQUIC Kind = "chameleon-quic"
	KindChameleonTCP  Kind = "chameleon-tcp"
	KindChameleonTLS Kind = "chameleon-tls"
	KindRelay        Kind = "relay"
)

// Candidate is one route the planner may try.
type Candidate struct {
	Name             string  `json:"name"`
	Kind             Kind    `json:"kind"`
	Endpoint         string  `json:"endpoint,omitempty"`
	Cost             float64 `json:"cost"`
	SupportsTCP      bool    `json:"supports_tcp"`
	SupportsUDP      bool    `json:"supports_udp"`
	RequiresExternal bool    `json:"requires_external,omitempty"`
}

// Context keeps route learning separate by network, destination and traffic class.
type Context struct {
	NetworkID    string `json:"network_id"`
	Destination  string `json:"destination"`
	TrafficClass string `json:"traffic_class"`
	Protocol     string `json:"protocol"`
}

// Observation is one measured route attempt.
type Observation struct {
	Context    Context       `json:"context"`
	Carrier    string        `json:"carrier"`
	Success    bool          `json:"success"`
	Latency    time.Duration `json:"latency"`
	Throughput float64       `json:"throughput"`
	Failure    string        `json:"failure,omitempty"`
	At         time.Time     `json:"at"`
}

// Stats is the compact long-term memory for one carrier.
type Stats struct {
	Attempts      uint64        `json:"attempts"`
	Successes     uint64        `json:"successes"`
	Failures      uint64        `json:"failures"`
	FailureStreak uint64        `json:"failure_streak"`
	AvgLatency    time.Duration `json:"avg_latency"`
	AvgThroughput float64       `json:"avg_throughput"`
	LastSuccess     time.Time     `json:"last_success,omitempty"`
	LastFailure     time.Time     `json:"last_failure,omitempty"`
	LastObservation time.Time     `json:"last_observation,omitempty"`
}

// Decision is the selected route and an explanation for diagnostics.
type Decision struct {
	Carrier    Candidate
	Score      float64
	Known      bool
	Reason     string
	Alternates []string
}

// Engine learns which carrier is useful in each network context.
type Engine struct {
	mu        sync.Mutex
	StorePath string                       `json:"-"`
	Entries   map[string]map[string]*Stats `json:"entries"`
}

// NewEngine creates or loads a carrier memory.
func NewEngine(storePath string) (*Engine, error) {
	e := &Engine{StorePath: storePath, Entries: make(map[string]map[string]*Stats)}
	if storePath == "" {
		return e, nil
	}
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir carrier store: %w", err)
	}
	data, err := os.ReadFile(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return e, nil
		}
		return nil, fmt.Errorf("read carrier store: %w", err)
	}
	if len(data) == 0 {
		return e, nil
	}
	if err := json.Unmarshal(data, e); err != nil {
		return nil, fmt.Errorf("unmarshal carrier store: %w", err)
	}
	e.StorePath = storePath
	if e.Entries == nil {
		e.Entries = make(map[string]map[string]*Stats)
	}
	return e, nil
}

// Observe saves one real route result.
func (e *Engine) Observe(obs Observation) error {
	if e == nil {
		return fmt.Errorf("carrier engine is nil")
	}
	if strings.TrimSpace(obs.Carrier) == "" {
		return fmt.Errorf("carrier name must not be empty")
	}
	if obs.At.IsZero() {
		obs.At = time.Now()
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	key := contextKey(obs.Context)
	if e.Entries[key] == nil {
		e.Entries[key] = make(map[string]*Stats)
	}
	stats := e.Entries[key][obs.Carrier]
	if stats == nil {
		stats = &Stats{}
		e.Entries[key][obs.Carrier] = stats
	}

	stats.Attempts++
	if obs.Success {
		stats.Successes++
		stats.FailureStreak = 0
		stats.LastSuccess = obs.At
	} else {
		stats.Failures++
		stats.FailureStreak++
		stats.LastFailure = obs.At
	}
	stats.LastObservation = obs.At
	stats.AvgLatency = runningDuration(stats.AvgLatency, obs.Latency, stats.Attempts)
	stats.AvgThroughput = runningFloat(stats.AvgThroughput, obs.Throughput, stats.Attempts)

	return e.saveLocked()
}

// Choose returns the best compatible carrier.
// With no history, the cheapest compatible carrier wins, usually direct.
func (e *Engine) Choose(ctx Context, candidates []Candidate) (Decision, error) {
	if e == nil {
		return Decision{}, fmt.Errorf("carrier engine is nil")
	}
	compatible := filterCompatible(ctx.Protocol, candidates)
	if len(compatible) == 0 {
		return Decision{}, fmt.Errorf("no compatible carrier for protocol %q", ctx.Protocol)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	statsByCarrier := e.Entries[contextKey(ctx)]
	type ranked struct {
		candidate Candidate
		score     float64
		known     bool
	}
	ranking := make([]ranked, 0, len(compatible))
	for _, candidate := range compatible {
		stats := statsByCarrier[candidate.Name]
		score, known := carrierScore(ctx.TrafficClass, candidate, stats, time.Now())
		ranking = append(ranking, ranked{candidate: candidate, score: score, known: known})
	}
	sort.SliceStable(ranking, func(i, j int) bool {
		if ranking[i].score == ranking[j].score {
			return ranking[i].candidate.Cost < ranking[j].candidate.Cost
		}
		return ranking[i].score > ranking[j].score
	})

	best := ranking[0]
	alternates := make([]string, 0, len(ranking)-1)
	for _, item := range ranking[1:] {
		alternates = append(alternates, item.candidate.Name)
	}

	reason := "no history: prefer the lowest-cost compatible carrier"
	if best.known {
		stats := statsByCarrier[best.candidate.Name]
		reason = fmt.Sprintf(
			"learned from %d attempts: %d success, %d failure, streak=%d",
			stats.Attempts, stats.Successes, stats.Failures, stats.FailureStreak,
		)
	}

	return Decision{
		Carrier:    best.candidate,
		Score:      best.score,
		Known:      best.known,
		Reason:     reason,
		Alternates: alternates,
	}, nil
}

// Snapshot returns a copy for status output and tests.
// HasEvidence reports whether any supplied carrier has observations in this exact
// context. Callers use it to decide when a small connection race is worth the
// extra work on a previously unknown network path.
func (e *Engine) HasEvidence(ctx Context, candidates []Candidate) bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	statsByCarrier := e.Entries[contextKey(ctx)]
	for _, candidate := range candidates {
		if stats := statsByCarrier[candidate.Name]; stats != nil && stats.Attempts > 0 {
			return true
		}
	}
	return false
}

func (e *Engine) Snapshot(ctx Context) map[string]Stats {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	src := e.Entries[contextKey(ctx)]
	out := make(map[string]Stats, len(src))
	for name, stats := range src {
		if stats != nil {
			out[name] = *stats
		}
	}
	return out
}

func (e *Engine) saveLocked() error {
	if e.StorePath == "" {
		return nil
	}
	payload, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal carrier store: %w", err)
	}
	if err := os.WriteFile(e.StorePath, payload, 0o644); err != nil {
		return fmt.Errorf("write carrier store: %w", err)
	}
	return nil
}

func contextKey(ctx Context) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(ctx.NetworkID)),
		strings.ToLower(strings.TrimSpace(ctx.Destination)),
		strings.ToLower(strings.TrimSpace(ctx.TrafficClass)),
		strings.ToLower(strings.TrimSpace(ctx.Protocol)),
	}
	defaults := []string{"unknown-network", "*", "default", "tcp"}
	for i := range parts {
		if parts[i] == "" {
			parts[i] = defaults[i]
		}
	}
	return strings.Join(parts, "|")
}

func filterCompatible(protocol string, candidates []Candidate) []Candidate {
	proto := strings.ToLower(strings.TrimSpace(protocol))
	if proto == "" {
		proto = "tcp"
	}
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if strings.TrimSpace(c.Name) == "" || c.Cost < 0 {
			continue
		}
		if proto == "udp" && !c.SupportsUDP {
			continue
		}
		if proto == "tcp" && !c.SupportsTCP {
			continue
		}
		out = append(out, c)
	}
	return out
}

func carrierScore(trafficClass string, candidate Candidate, stats *Stats, now time.Time) (float64, bool) {
	baseline := 0.25 - candidate.Cost
	if stats == nil || stats.Attempts == 0 {
		return baseline, false
	}

	successRate := float64(stats.Successes) / float64(stats.Attempts)
	learned := successRate*7.0 - float64(stats.FailureStreak)*1.8 - candidate.Cost

	latencyPenalty := math.Min(float64(stats.AvgLatency)/float64(time.Second), 2.5)
	throughputBoost := math.Min(stats.AvgThroughput/(8*1024*1024), 2.0)

	switch strings.ToLower(trafficClass) {
	case "interactive", "realtime":
		learned -= latencyPenalty * 2.2
		learned += throughputBoost * 0.25
	case "streaming":
		learned -= latencyPenalty * 0.7
		learned += throughputBoost * 1.5
	case "bulk":
		learned -= latencyPenalty * 0.3
		learned += throughputBoost * 2.0
	default:
		learned -= latencyPenalty
		learned += throughputBoost
	}

	freshness := evidenceFreshness(stats, now)
	return baseline + freshness*(learned-baseline), true
}

func evidenceFreshness(stats *Stats, now time.Time) float64 {
	if stats == nil {
		return 0
	}
	last := stats.LastObservation
	if last.IsZero() {
		if stats.LastSuccess.After(stats.LastFailure) {
			last = stats.LastSuccess
		} else {
			last = stats.LastFailure
		}
	}
	if last.IsZero() || now.IsZero() {
		return 1
	}
	age := now.Sub(last)
	if age <= 0 {
		return 1
	}

	// One day is a deliberate half-life: network filtering can change quickly.
	const halfLife = 24 * time.Hour
	return math.Exp(-math.Ln2 * float64(age) / float64(halfLife))
}

func runningDuration(current, next time.Duration, count uint64) time.Duration {
	if count <= 1 {
		return next
	}
	return time.Duration((int64(current)*int64(count-1) + int64(next)) / int64(count))
}

func runningFloat(current, next float64, count uint64) float64 {
	if count <= 1 {
		return next
	}
	return (current*float64(count-1) + next) / float64(count)
}
