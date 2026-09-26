package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientapp"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
)

func testRuntime(t *testing.T) *clientapp.Runtime {
	t.Helper()
	cfg := clientconfig.Default()
	cfg.StateDir = t.TempDir()
	runtime, err := clientapp.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestPresetAPIUpdatesRuntimeAndConfig(t *testing.T) {
	runtime := testRuntime(t)
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := clientconfig.Save(configPath, runtime.ConfigSnapshot()); err != nil {
		t.Fatal(err)
	}

	server := &Server{Runtime: runtime, ConfigPath: configPath}
	req := httptest.NewRequest(http.MethodPost, "/api/preset", bytes.NewBufferString(`{"preset":"gaming"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	if got := runtime.ConfigSnapshot().Preset; got != "gaming" {
		t.Fatalf("runtime preset=%q", got)
	}
	saved, err := clientconfig.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Preset != "gaming" {
		t.Fatalf("saved preset=%q", saved.Preset)
	}
}

func TestStatusNeverContainsPSK(t *testing.T) {
	runtime := testRuntime(t)
	server := &Server{Runtime: runtime}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rec, req)

	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if _, exists := status["psk"]; exists {
		t.Fatal("status must not expose PSK")
	}
}

func TestControlServeRejectsRemoteBind(t *testing.T) {
	runtime := testRuntime(t)
	server := &Server{Runtime: runtime}
	if err := server.Serve(context.Background(), "0.0.0.0:8765"); err == nil {
		t.Fatal("expected non-loopback control bind rejection")
	}
}

func TestLocalOnlyRejectsRemoteClient(t *testing.T) {
	called := false
	handler := localOnly(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = net.JoinHostPort("192.0.2.10", "1234")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if called {
		t.Fatal("remote client reached local control handler")
	}
}
