package adaptive

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"time"
)

// Observation captures one learned transport experience.
type Observation struct {
	Profile    string
	Success    bool
	Latency    time.Duration
	Loss       float64
	Throughput float64
	Load       float64
	SessionID  string
	Epoch      int64
	At         time.Time
}

// Decision is the learned best route for the next send window.
type Decision struct {
	Profile    string
	Reason     string
	Confidence float64
}

// Learner is a lightweight adaptive policy engine.
type Learner struct {
	mu           sync.Mutex
	history      map[string][]Observation
	lastDecision Decision
	storePath    string
}

// NewLearner creates a bounded adaptive policy memory.
func NewLearner() *Learner {
	return &Learner{
		history: make(map[string][]Observation),
	}
}

// Observe records one experience and updates the internal scorecard.
func (l *Learner) Observe(obs Observation) {
	if obs.Profile == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.history[obs.Profile] = append(l.history[obs.Profile], obs)

	if len(l.history[obs.Profile]) > 64 {
		l.history[obs.Profile] = l.history[obs.Profile][len(l.history[obs.Profile])-64:]
	}

	if l.storePath != "" {
		_ = l.saveLocked()
	}
}

// Decide returns the best known decision and keeps the winning profile in memory.
func (l *Learner) Decide() Decision {
	l.mu.Lock()
	defer l.mu.Unlock()

	bestProfile := l.BestProfileLocked()
	if bestProfile == "" {
		bestProfile = "webrtc"
	}

	confidence := l.score(bestProfile)
	l.lastDecision = Decision{
		Profile:    bestProfile,
		Reason:     fmt.Sprintf("adaptive score from %d observations (confidence %.2f)", len(l.history[bestProfile]), confidence),
		Confidence: confidence,
	}

	return l.lastDecision
}

// BestProfile returns the best profile according to a lightweight score heuristic.
func (l *Learner) BestProfile() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.BestProfileLocked()
}

// HasHistory reports whether the learner already has persisted observations.
func (l *Learner) HasHistory() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, observations := range l.history {
		if len(observations) > 0 {
			return true
		}
	}
	return false
}

// SetStorePath configures a JSON persistence file for learned observations.
func (l *Learner) SetStorePath(path string) error {
	if path == "" {
		return fmt.Errorf("store path must not be empty")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.storePath = path

	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read store: %w", err)
		}
		if len(data) == 0 {
			return nil
		}
		var history map[string][]Observation
		if err := json.Unmarshal(data, &history); err != nil {
			return fmt.Errorf("unmarshal store: %w", err)
		}
		l.history = history
	}

	return nil
}

// Save persists the current learner memory to its configured JSON file.
func (l *Learner) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.saveLocked()
}

func (l *Learner) saveLocked() error {
	if l.storePath == "" {
		return nil
	}

	payload, err := json.Marshal(l.history)
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}
	if err := os.WriteFile(l.storePath, payload, 0o644); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}

func (l *Learner) BestProfileLocked() string {
	if len(l.history) == 0 {
		return "webrtc"
	}

	profiles := make([]string, 0, len(l.history))
	for profile := range l.history {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)

	bestProfile := "webrtc"
	bestScore := -math.MaxFloat64
	for _, profile := range profiles {
		score := l.score(profile)
		if score > bestScore {
			bestScore = score
			bestProfile = profile
		}
	}

	return bestProfile
}

func (l *Learner) score(profile string) float64 {
	observations := l.history[profile]
	if len(observations) == 0 {
		return 0
	}

	latest := time.Time{}
	for _, obs := range observations {
		if obs.At.After(latest) {
			latest = obs.At
		}
	}
	if latest.IsZero() {
		latest = time.Now()
	}

	successCount := 0
	streak := 0
	bestStreak := 0
	score := 0.0
	for _, obs := range observations {
		if obs.Success {
			successCount++
			streak++
			if streak > bestStreak {
				bestStreak = streak
			}
			score += 1.6
		} else {
			if streak > bestStreak {
				bestStreak = streak
			}
			streak = 0
			score -= 1.2
		}

		latencyFactor := 1.0 / (1.0 + float64(obs.Latency)/float64(time.Millisecond))
		lossPenalty := obs.Loss * 3.0
		loadPenalty := obs.Load * 0.5
		throughputBoost := math.Min(obs.Throughput/1000.0, 4.0)
		recencyBoost := 0.0
		if !obs.At.IsZero() {
			delta := latest.Sub(obs.At)
			recencyBoost = math.Max(0.0, 1.0-(float64(delta)/float64(30*time.Minute)))
		}
		stabilityBoost := math.Min(float64(bestStreak)*0.12, 1.2)

		score += latencyFactor*2.0 + throughputBoost + recencyBoost + stabilityBoost - lossPenalty - loadPenalty
	}

	successRate := float64(successCount) / float64(len(observations))
	return (score / float64(len(observations))) + (successRate * 0.75) + (math.Min(float64(bestStreak)*0.15, 1.0))
}
