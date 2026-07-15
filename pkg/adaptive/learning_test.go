package adaptive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAdaptiveLearnerPromotesBestPath(t *testing.T) {
	learner := NewLearner()

	learner.Observe(Observation{
		Profile:    "webrtc",
		Success:    true,
		Latency:    5 * time.Millisecond,
		Loss:       0.01,
		Throughput: 1200,
		Load:       0.4,
	})
	learner.Observe(Observation{
		Profile:    "http3",
		Success:    true,
		Latency:    2 * time.Millisecond,
		Loss:       0.00,
		Throughput: 1800,
		Load:       0.3,
	})
	learner.Observe(Observation{
		Profile:    "gaming",
		Success:    false,
		Latency:    18 * time.Millisecond,
		Loss:       0.12,
		Throughput: 600,
		Load:       0.6,
	})

	best := learner.BestProfile()
	if best != "http3" {
		t.Fatalf("unexpected best profile: %s", best)
	}
}

func TestAdaptiveLearnerPersistsDecision(t *testing.T) {
	learner := NewLearner()
	learner.Observe(Observation{
		Profile:    "webrtc",
		Success:    true,
		Latency:    3 * time.Millisecond,
		Loss:       0.01,
		Throughput: 1500,
		Load:       0.35,
	})

	decision := learner.Decide()
	if decision.Profile != "webrtc" {
		t.Fatalf("unexpected persisted decision: %s", decision.Profile)
	}
}

func TestAdaptiveLearnerCanPersistAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "learner_state.json")

	learner := NewLearner()
	if err := learner.SetStorePath(storePath); err != nil {
		t.Fatalf("SetStorePath returned error: %v", err)
	}
	learner.Observe(Observation{
		Profile:    "http3",
		Success:    true,
		Latency:    2 * time.Millisecond,
		Loss:       0.0,
		Throughput: 2000,
		Load:       0.3,
	})
	if err := learner.Save(); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("expected persisted state file: %v", err)
	}

	restored := NewLearner()
	if err := restored.SetStorePath(storePath); err != nil {
		t.Fatalf("SetStorePath on restored learner returned error: %v", err)
	}
	if restored.BestProfile() != "http3" {
		t.Fatalf("unexpected restored best profile: %s", restored.BestProfile())
	}
}
