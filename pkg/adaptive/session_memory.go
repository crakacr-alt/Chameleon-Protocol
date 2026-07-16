package adaptive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionMemory stores the longest-running evidence for a transport session.
type SessionMemory struct {
	StorePath string                   `json:"store_path"`
	Profiles  map[string]ProfileMemory `json:"profiles"`
}

// ProfileMemory stores session-scoped profile performance evidence.
type ProfileMemory struct {
	SessionID     string        `json:"session_id"`
	LastObserved  time.Time     `json:"last_observed"`
	SuccessCount  int           `json:"success_count"`
	FailureCount  int           `json:"failure_count"`
	AvgLatency    time.Duration `json:"avg_latency"`
	AvgThroughput float64       `json:"avg_throughput"`
}

// NewSessionMemory creates a persistent session memory container.
func NewSessionMemory(storePath string) (*SessionMemory, error) {
	if storePath == "" {
		return nil, fmt.Errorf("store path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	mem := &SessionMemory{StorePath: storePath, Profiles: make(map[string]ProfileMemory)}
	if data, err := os.ReadFile(storePath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, mem); err != nil {
			return nil, fmt.Errorf("unmarshal session memory: %w", err)
		}
		if mem.Profiles == nil {
			mem.Profiles = make(map[string]ProfileMemory)
		}
	}
	return mem, nil
}

// Observe records one session observation into long-term memory.
func (m *SessionMemory) Observe(obs Observation) error {
	if m == nil || obs.Profile == "" {
		return nil
	}
	profile := strings.ToLower(obs.Profile)
	entry := m.Profiles[profile]
	entry.SessionID = obs.SessionID
	entry.LastObserved = obs.At
	if obs.Success {
		entry.SuccessCount++
	} else {
		entry.FailureCount++
	}
	entry.AvgLatency = averageDuration(entry.AvgLatency, obs.Latency, entry.SuccessCount+entry.FailureCount)
	entry.AvgThroughput = averageFloat64(entry.AvgThroughput, obs.Throughput, entry.SuccessCount+entry.FailureCount)
	m.Profiles[profile] = entry
	return m.Save()
}

// BestProfile returns the most successful profile from the saved session memory.
func (m *SessionMemory) BestProfile() string {
	if m == nil || len(m.Profiles) == 0 {
		return "webrtc"
	}
	profiles := make([]string, 0, len(m.Profiles))
	for profile := range m.Profiles {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)

	bestProfile := "webrtc"
	bestScore := -1.0
	for _, profile := range profiles {
		entry := m.Profiles[profile]
		weight := float64(entry.SuccessCount) - float64(entry.FailureCount)*1.2
		if weight > bestScore {
			bestScore = weight
			bestProfile = profile
		}
	}
	return bestProfile
}

// Save persists the session memory to disk.
func (m *SessionMemory) Save() error {
	if m == nil || m.StorePath == "" {
		return nil
	}
	payload, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session memory: %w", err)
	}
	if err := os.WriteFile(m.StorePath, payload, 0o644); err != nil {
		return fmt.Errorf("write session memory: %w", err)
	}
	return nil
}

func averageDuration(current, next time.Duration, count int) time.Duration {
	if count <= 1 {
		return next
	}
	return (current*time.Duration(count-1) + next) / time.Duration(count)
}

func averageFloat64(current, next float64, count int) float64 {
	if count <= 1 {
		return next
	}
	return (current*float64(count-1) + next) / float64(count)
}
