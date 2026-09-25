package adaptive

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

// TrafficClass lets Chameleon value routes differently for web, games, video
// and bulk transfers. A low-latency path is not always the highest-throughput path.
type TrafficClass string

const (
	TrafficGeneric     TrafficClass = "generic"
	TrafficWeb         TrafficClass = "web"
	TrafficInteractive TrafficClass = "interactive"
	TrafficStreaming   TrafficClass = "streaming"
	TrafficBulk        TrafficClass = "bulk"
)

// PathCandidate describes one way to reach a logical endpoint.
// Carrier is transport-level (udp/quic/tcp-tls/http2/relay/derp).
// Strategy is the optional DPI policy name (direct/split/tls-record/...).
type PathCandidate struct {
	Carrier  string
	Endpoint string
	Strategy string
	BaseCost float64
}

// PathObservation is one measurement from a live flow or a cheap probe.
type PathObservation struct {
	NetworkID   string
	Destination string
	Traffic     TrafficClass
	Candidate   PathCandidate
	Success     bool
	Latency     time.Duration
	Jitter      time.Duration
	Loss        float64
	Throughput  float64
	Overhead    float64
	At          time.Time
}

// PathStats is compact long-term state for one context/candidate combination.
type PathStats struct {
	Samples             int
	Successes           int
	Failures            int
	ConsecutiveFailures int
	LatencyEWMA         time.Duration
	JitterEWMA          time.Duration
	LossEWMA            float64
	ThroughputEWMA      float64
	OverheadEWMA        float64
	LastSeen            time.Time
	LastSuccess         time.Time
}

// RankedCandidate is the result of scoring one route.
type RankedCandidate struct {
	Candidate PathCandidate
	Score     float64
	Known     bool
	Samples   int
	Reason    string
}

// PathLearner is deliberately lightweight: updates are O(1), ranking is O(N),
// and persistence is a tiny JSON file. No neural network or cloud service is needed.
type PathLearner struct {
	mu        sync.Mutex
	StorePath string
	Alpha     float64
	Entries   map[string]PathStats
}

// NewPathLearner creates a persistent learner. Empty storePath means memory-only.
func NewPathLearner(storePath string) (*PathLearner, error) {
	l := &PathLearner{StorePath: storePath, Alpha: 0.25, Entries: make(map[string]PathStats)}
	if storePath == "" {
		return l, nil
	}
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return nil, fmt.Errorf("create learner directory: %w", err)
	}
	if data, err := os.ReadFile(storePath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, l); err != nil {
			return nil, fmt.Errorf("load path learner: %w", err)
		}
		l.StorePath = storePath
		if l.Alpha <= 0 || l.Alpha > 1 {
			l.Alpha = 0.25
		}
		if l.Entries == nil {
			l.Entries = make(map[string]PathStats)
		}
	}
	return l, nil
}

// Observe folds one fresh measurement into the route memory.
func (l *PathLearner) Observe(obs PathObservation) error {
	if l == nil {
		return nil
	}
	if strings.TrimSpace(obs.Destination) == "" || strings.TrimSpace(obs.Candidate.Carrier) == "" {
		return fmt.Errorf("destination and carrier must not be empty")
	}
	if obs.NetworkID == "" {
		obs.NetworkID = "default"
	}
	if obs.Traffic == "" {
		obs.Traffic = TrafficGeneric
	}
	if obs.Candidate.Strategy == "" {
		obs.Candidate.Strategy = "direct"
	}
	if obs.At.IsZero() {
		obs.At = time.Now()
	}
	obs.Loss = clamp(obs.Loss, 0, 1)
	obs.Overhead = math.Max(0, obs.Overhead)

	l.mu.Lock()
	defer l.mu.Unlock()

	key := pathKey(obs.NetworkID, obs.Destination, obs.Traffic, obs.Candidate)
	s := l.Entries[key]
	s.Samples++
	if obs.Success {
		s.Successes++
		s.ConsecutiveFailures = 0
		s.LastSuccess = obs.At
	} else {
		s.Failures++
		s.ConsecutiveFailures++
	}
	s.LatencyEWMA = ewmaDuration(s.LatencyEWMA, obs.Latency, l.Alpha, s.Samples)
	s.JitterEWMA = ewmaDuration(s.JitterEWMA, obs.Jitter, l.Alpha, s.Samples)
	s.LossEWMA = ewmaFloat(s.LossEWMA, obs.Loss, l.Alpha, s.Samples)
	s.ThroughputEWMA = ewmaFloat(s.ThroughputEWMA, math.Max(0, obs.Throughput), l.Alpha, s.Samples)
	s.OverheadEWMA = ewmaFloat(s.OverheadEWMA, obs.Overhead, l.Alpha, s.Samples)
	s.LastSeen = obs.At
	l.Entries[key] = s
	return l.saveLocked()
}

// Rank orders available routes for one network + destination + traffic class.
// Unknown routes get a small exploration bonus so Chameleon can learn new paths.
func (l *PathLearner) Rank(networkID, destination string, traffic TrafficClass, candidates []PathCandidate) []RankedCandidate {
	if l == nil {
		return nil
	}
	if networkID == "" {
		networkID = "default"
	}
	if traffic == "" {
		traffic = TrafficGeneric
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]RankedCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Strategy == "" {
			c.Strategy = "direct"
		}
		s, ok := l.Entries[pathKey(networkID, destination, traffic, c)]
		reason := "unseen route; exploration candidate"
		if ok {
			rate := float64(s.Successes) / float64(maxInt(1, s.Samples))
			reason = fmt.Sprintf("%d samples, %.0f%% success, %s latency, %.1f%% loss",
				s.Samples, rate*100, s.LatencyEWMA.Round(time.Millisecond), s.LossEWMA*100)
		}
		out = append(out, RankedCandidate{
			Candidate: c,
			Score:     scorePath(traffic, c, s, ok),
			Known:     ok,
			Samples:   s.Samples,
			Reason:    reason,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Candidate.BaseCost < out[j].Candidate.BaseCost
		}
		return out[i].Score > out[j].Score
	})
	return out
}

// ShouldSwitch provides hysteresis. A healthy route is retained unless another
// route is materially better; repeated failures bypass that protection.
func (l *PathLearner) ShouldSwitch(networkID, destination string, traffic TrafficClass, current, next PathCandidate, minGain float64) bool {
	if l == nil {
		return false
	}
	if minGain <= 0 {
		minGain = 8
	}
	ranked := l.Rank(networkID, destination, traffic, []PathCandidate{current, next})
	var currentScore, nextScore float64
	for _, r := range ranked {
		if sameCandidate(r.Candidate, current) {
			currentScore = r.Score
		}
		if sameCandidate(r.Candidate, next) {
			nextScore = r.Score
		}
	}

	l.mu.Lock()
	currentStats := l.Entries[pathKey(networkID, destination, traffic, current)]
	l.mu.Unlock()
	return currentStats.ConsecutiveFailures >= 2 || nextScore-currentScore >= minGain
}

// Stats returns a copy of the remembered state for a candidate.
func (l *PathLearner) Stats(networkID, destination string, traffic TrafficClass, c PathCandidate) (PathStats, bool) {
	if l == nil {
		return PathStats{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.Entries[pathKey(networkID, destination, traffic, c)]
	return s, ok
}

func (l *PathLearner) saveLocked() error {
	if l.StorePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encode path learner: %w", err)
	}
	tmp := l.StorePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write path learner: %w", err)
	}
	if err := os.Rename(tmp, l.StorePath); err != nil {
		return fmt.Errorf("replace path learner: %w", err)
	}
	return nil
}

func scorePath(traffic TrafficClass, c PathCandidate, s PathStats, known bool) float64 {
	if !known || s.Samples == 0 {
		return 38 - c.BaseCost
	}
	successRate := float64(s.Successes) / float64(maxInt(1, s.Samples))
	latencyMS := float64(s.LatencyEWMA) / float64(time.Millisecond)
	jitterMS := float64(s.JitterEWMA) / float64(time.Millisecond)

	score := successRate*60 - s.LossEWMA*45 - float64(s.ConsecutiveFailures)*15
	score -= c.BaseCost + math.Min(s.OverheadEWMA*12, 18)
	score += math.Min(math.Log1p(s.ThroughputEWMA/1024)*4, 18)

	switch traffic {
	case TrafficInteractive:
		score -= math.Min(latencyMS/7, 24)
		score -= math.Min(jitterMS/4, 18)
	case TrafficStreaming:
		score -= math.Min(latencyMS/30, 10)
		score += math.Min(math.Log1p(s.ThroughputEWMA/(256*1024))*8, 18)
	case TrafficBulk:
		score -= math.Min(latencyMS/80, 6)
		score += math.Min(math.Log1p(s.ThroughputEWMA/(512*1024))*10, 20)
	default:
		score -= math.Min(latencyMS/18, 15)
		score -= math.Min(jitterMS/10, 8)
	}
	score += 7 / math.Sqrt(float64(s.Samples))
	return score
}

func pathKey(networkID, destination string, traffic TrafficClass, c PathCandidate) string {
	return strings.Join([]string{
		cleanKey(networkID), cleanKey(destination), cleanKey(string(traffic)),
		cleanKey(c.Carrier), cleanKey(c.Endpoint), cleanKey(c.Strategy),
	}, "|")
}

func cleanKey(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func sameCandidate(a, b PathCandidate) bool {
	return cleanKey(a.Carrier) == cleanKey(b.Carrier) &&
		cleanKey(a.Endpoint) == cleanKey(b.Endpoint) &&
		cleanKey(a.Strategy) == cleanKey(b.Strategy)
}

func ewmaFloat(current, next, alpha float64, samples int) float64 {
	if samples <= 1 {
		return next
	}
	return current*(1-alpha) + next*alpha
}

func ewmaDuration(current, next time.Duration, alpha float64, samples int) time.Duration {
	if next < 0 {
		next = 0
	}
	if samples <= 1 {
		return next
	}
	return time.Duration(float64(current)*(1-alpha) + float64(next)*alpha)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
