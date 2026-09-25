package dpi

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

// Context separates experience from different networks and destinations.
// A strategy that works on home Wi-Fi must not automatically become the
// preferred strategy on a mobile network.
type Context struct {
	NetworkID    string `json:"network_id"`
	Destination  string `json:"destination"`
	TrafficClass string `json:"traffic_class"`
}

// Observation is one real result collected after trying a strategy.
type Observation struct {
	Context    Context       `json:"context"`
	Strategy   string        `json:"strategy"`
	Success    bool          `json:"success"`
	Latency    time.Duration `json:"latency"`
	Throughput float64       `json:"throughput"`
	Failure    string        `json:"failure,omitempty"`
	At         time.Time     `json:"at"`
}

// StrategyStats is the small amount of long-term state we keep.
// It is deliberately simple: no neural network and no cloud service.
type StrategyStats struct {
	Attempts      uint64        `json:"attempts"`
	Successes     uint64        `json:"successes"`
	Failures      uint64        `json:"failures"`
	FailureStreak uint64        `json:"failure_streak"`
	AvgLatency    time.Duration `json:"avg_latency"`
	AvgThroughput float64       `json:"avg_throughput"`
	LastSuccess   time.Time     `json:"last_success,omitempty"`
	LastFailure   time.Time     `json:"last_failure,omitempty"`
}

// Decision explains which strategy was selected and why.
type Decision struct {
	Strategy   Strategy
	Score      float64
	Known      bool
	Reason     string
	Alternates []string
}

// Engine learns a strategy independently for each network/destination/class.
type Engine struct {
	mu        sync.Mutex
	StorePath string                               `json:"-"`
	Entries   map[string]map[string]*StrategyStats `json:"entries"`
}

// NewEngine creates an in-memory engine or loads a JSON store when path is set.
func NewEngine(storePath string) (*Engine, error) {
	e := &Engine{
		StorePath: storePath,
		Entries:   make(map[string]map[string]*StrategyStats),
	}
	if storePath == "" {
		return e, nil
	}

	dir := filepath.Dir(storePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir dpi store: %w", err)
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return e, nil
		}
		return nil, fmt.Errorf("read dpi store: %w", err)
	}
	if len(data) == 0 {
		return e, nil
	}
	if err := json.Unmarshal(data, e); err != nil {
		return nil, fmt.Errorf("unmarshal dpi store: %w", err)
	}
	e.StorePath = storePath
	if e.Entries == nil {
		e.Entries = make(map[string]map[string]*StrategyStats)
	}
	return e, nil
}

// Observe adds one result and persists it when a store path is configured.
func (e *Engine) Observe(obs Observation) error {
	if e == nil {
		return fmt.Errorf("dpi engine is nil")
	}
	if strings.TrimSpace(obs.Strategy) == "" {
		return fmt.Errorf("strategy must not be empty")
	}
	if obs.At.IsZero() {
		obs.At = time.Now()
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	key := contextKey(obs.Context)
	if e.Entries[key] == nil {
		e.Entries[key] = make(map[string]*StrategyStats)
	}
	stats := e.Entries[key][obs.Strategy]
	if stats == nil {
		stats = &StrategyStats{}
		e.Entries[key][obs.Strategy] = stats
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
	stats.AvgLatency = runningDuration(stats.AvgLatency, obs.Latency, stats.Attempts)
	stats.AvgThroughput = runningFloat(stats.AvgThroughput, obs.Throughput, stats.Attempts)

	return e.saveLocked()
}

// Choose selects the best candidate for this exact network context.
// With no evidence it prefers the cheapest strategy, normally direct.
func (e *Engine) Choose(ctx Context, candidates []Strategy) (Decision, error) {
	if e == nil {
		return Decision{}, fmt.Errorf("dpi engine is nil")
	}
	if len(candidates) == 0 {
		return Decision{}, fmt.Errorf("no dpi strategies supplied")
	}
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return Decision{}, err
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	statsByStrategy := e.Entries[contextKey(ctx)]
	type ranked struct {
		strategy Strategy
		score    float64
		known    bool
	}
	ranking := make([]ranked, 0, len(candidates))
	for _, candidate := range candidates {
		stats := statsByStrategy[candidate.Name]
		score, known := strategyScore(candidate, stats)
		ranking = append(ranking, ranked{strategy: candidate, score: score, known: known})
	}

	sort.SliceStable(ranking, func(i, j int) bool {
		if ranking[i].score == ranking[j].score {
			return ranking[i].strategy.Cost < ranking[j].strategy.Cost
		}
		return ranking[i].score > ranking[j].score
	})

	best := ranking[0]
	alternates := make([]string, 0, len(ranking)-1)
	for _, item := range ranking[1:] {
		alternates = append(alternates, item.strategy.Name)
	}

	reason := "no history: use the lowest-cost strategy first"
	if best.known {
		stats := statsByStrategy[best.strategy.Name]
		reason = fmt.Sprintf(
			"learned from %d attempts: %d success, %d failure, streak=%d",
			stats.Attempts, stats.Successes, stats.Failures, stats.FailureStreak,
		)
	}

	return Decision{
		Strategy:   best.strategy,
		Score:      best.score,
		Known:      best.known,
		Reason:     reason,
		Alternates: alternates,
	}, nil
}

// Snapshot returns a copy useful for diagnostics and tests.
func (e *Engine) Snapshot(ctx Context) map[string]StrategyStats {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	src := e.Entries[contextKey(ctx)]
	out := make(map[string]StrategyStats, len(src))
	for name, stats := range src {
		if stats != nil {
			out[name] = *stats
		}
	}
	return out
}

// RecentlyExhausted reports whether every supplied strategy has recently
// failed in this exact network/destination context and none has recovered
// after its last failure. It is used as a short-lived circuit breaker for
// direct traffic so the caller can try another carrier instead of cycling
// the same DPI strategies forever.
func (e *Engine) RecentlyExhausted(ctx Context, candidates []Strategy, now time.Time, window time.Duration) bool {
	if e == nil || len(candidates) == 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	if window <= 0 {
		window = 10 * time.Minute
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	statsByStrategy := e.Entries[contextKey(ctx)]
	for _, candidate := range candidates {
		stats := statsByStrategy[candidate.Name]
		if stats == nil || stats.Attempts == 0 || stats.LastFailure.IsZero() {
			return false
		}
		if stats.LastSuccess.After(stats.LastFailure) {
			return false
		}
		age := now.Sub(stats.LastFailure)
		if age < 0 || age > window {
			return false
		}
	}
	return true
}

// Save writes the current memory to disk.
func (e *Engine) Save() error {
	if e == nil {
		return fmt.Errorf("dpi engine is nil")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.saveLocked()
}

func (e *Engine) saveLocked() error {
	if e.StorePath == "" {
		return nil
	}
	payload, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dpi store: %w", err)
	}
	if err := os.WriteFile(e.StorePath, payload, 0o644); err != nil {
		return fmt.Errorf("write dpi store: %w", err)
	}
	return nil
}

func contextKey(ctx Context) string {
	network := strings.ToLower(strings.TrimSpace(ctx.NetworkID))
	destination := strings.ToLower(strings.TrimSpace(ctx.Destination))
	class := strings.ToLower(strings.TrimSpace(ctx.TrafficClass))
	if network == "" {
		network = "unknown-network"
	}
	if destination == "" {
		destination = "*"
	}
	if class == "" {
		class = "default"
	}
	return network + "|" + destination + "|" + class
}

func strategyScore(strategy Strategy, stats *StrategyStats) (float64, bool) {
	if stats == nil || stats.Attempts == 0 {
		// Untried expensive strategies should not beat direct traffic by default.
		return 0.20 - strategy.Cost, false
	}

	successRate := float64(stats.Successes) / float64(stats.Attempts)
	score := successRate*6.0 - float64(stats.FailureStreak)*1.6 - strategy.Cost

	if stats.AvgLatency > 0 {
		score -= math.Min(float64(stats.AvgLatency)/float64(500*time.Millisecond), 2.0)
	}
	if stats.AvgThroughput > 0 {
		// Throughput is bytes/second. The boost is intentionally capped so
		// one large transfer cannot permanently dominate the decision.
		score += math.Min(stats.AvgThroughput/(5*1024*1024), 1.5)
	}

	return score, true
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
