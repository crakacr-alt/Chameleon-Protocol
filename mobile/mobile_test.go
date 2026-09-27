package mobile

import (
	"strings"
	"testing"
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
