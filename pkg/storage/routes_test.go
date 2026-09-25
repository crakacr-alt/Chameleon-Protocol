package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpsertPersistsWithoutDeadlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- store.Upsert("example", "webrtc", 123)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Upsert returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Upsert deadlocked while persisting the store")
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	route, ok := reloaded.Get("example")
	if !ok || route.Profile != "webrtc" || route.LastUsed != 123 {
		t.Fatalf("unexpected persisted route: %#v, ok=%t", route, ok)
	}
}

func TestStorePathIsRuntimeOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.json")
	if err := os.WriteFile(path, []byte(`{"path":"/tmp/redirected.json","routes":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Path != path {
		t.Fatalf("persisted JSON overrode runtime path: got %q want %q", store.Path, path)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"path"`) {
		t.Fatalf("runtime path must not be persisted: %s", data)
	}
}
