package adaptive

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionMemoryPersistsAndPromotesBestProfile(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "session_memory.json")

	mem, err := NewSessionMemory(storePath)
	if err != nil {
		t.Fatalf("NewSessionMemory returned error: %v", err)
	}

	if err := mem.Observe(Observation{
		Profile:    "http3",
		Success:    true,
		Latency:    3 * time.Millisecond,
		Throughput: 1800,
		Load:       0.25,
		SessionID:  "session-1",
		At:         time.Now(),
	}); err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}

	if got := mem.BestProfile(); got != "http3" {
		t.Fatalf("unexpected best profile from session memory: %s", got)
	}
}
