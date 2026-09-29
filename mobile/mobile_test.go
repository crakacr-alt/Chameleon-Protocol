package mobile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
)

func TestBuildConfigFromServerProfile(t *testing.T) {
	profile := `
CHAMELEON_SERVER=server.example:443
CHAMELEON_QUIC_SERVER=server.example:443
CHAMELEON_TLS_SERVER=server.example:443
CHAMELEON_TUNNEL_PSK=0123456789abcdef
CHAMELEON_TLS_FINGERPRINT=aabbcc
`
	cfg := BuildConfig(profile, "/tmp/chameleon", "proxy")
	if strings.HasPrefix(cfg, "ERROR:") {
		t.Fatal(cfg)
	}
	if err := ValidateConfig(cfg); err != "" {
		t.Fatal(err)
	}
}

func TestRejectUnknownMode(t *testing.T) {
	got := BuildConfig("", "", "mystery")
	if !strings.HasPrefix(got, "ERROR:") {
		t.Fatalf("expected error, got %q", got)
	}
}

func TestRuntimeOwnerPreventsStaleStop(t *testing.T) {
	cfg := clientconfig.Default()
	cfg.Listen = "127.0.0.1:0"
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if got := StartOwned(string(data), "sidecar"); got != "" {
		t.Fatal(got)
	}
	if Owner() != "sidecar" || !Running() || !ListenerReady() {
		t.Fatal("sidecar runtime did not become ready")
	}

	StopOwned("vpn")
	if !Running() || Owner() != "sidecar" {
		t.Fatal("stale owner stopped the active runtime")
	}

	StopOwned("sidecar")
	if Running() || Owner() != "" {
		t.Fatal("sidecar runtime did not stop")
	}
}
