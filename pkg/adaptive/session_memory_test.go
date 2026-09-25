package adaptive

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestSessionMemoryConcurrentAccess(t *testing.T) {
	mem, err := NewSessionMemory(filepath.Join(t.TempDir(), "session_memory.json"))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func(index int) {
			defer wg.Done()
			profile := "webrtc"
			if index%2 == 0 {
				profile = "http3"
			}
			if err := mem.Observe(Observation{
				Profile: profile, Success: true, SessionID: "concurrent", At: time.Now(),
			}); err != nil {
				t.Errorf("Observe returned error: %v", err)
			}
		}(i)
		go func() {
			defer wg.Done()
			_ = mem.BestProfile()
		}()
	}
	wg.Wait()
}

func TestSessionMemoryPathIsRuntimeOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	if err := os.WriteFile(path, []byte(`{"store_path":"/tmp/redirected.json","profiles":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	mem, err := NewSessionMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	if mem.StorePath != path {
		t.Fatalf("persisted JSON overrode runtime path: got %q want %q", mem.StorePath, path)
	}
	if err := mem.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "store_path") {
		t.Fatalf("runtime path must not be persisted: %s", data)
	}
}
