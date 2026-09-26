package clientconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImportServerProfile(t *testing.T) {
	cfg, err := ImportProfile(strings.NewReader(`
CHAMELEON_SERVER=server.example:443
CHAMELEON_QUIC_SERVER=server.example:443
CHAMELEON_TLS_SERVER=server.example:443
CHAMELEON_TUNNEL_PSK=0123456789abcdef
CHAMELEON_TLS_FINGERPRINT=aabbcc
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QUICServer != "server.example:443" || cfg.TLSServer != "server.example:443" {
		t.Fatalf("unexpected endpoints: %+v", cfg)
	}
	if cfg.PSK != "0123456789abcdef" {
		t.Fatal("PSK not imported")
	}
}

func TestSaveLoadPermissionsAndDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Default()
	cfg.QUICServer = "server:443"
	cfg.PSK = "0123456789abcdef"
	cfg.DirectCooldown = Duration(5 * time.Minute)

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config is too permissive: %o", info.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.DirectCooldown.Duration() != 5*time.Minute {
		t.Fatalf("duration changed: %s", got.DirectCooldown.Duration())
	}
}

func TestRejectFutureSchema(t *testing.T) {
	cfg := Default()
	cfg.SchemaVersion = SchemaVersion + 1
	if err := cfg.Migrate(); err == nil {
		t.Fatal("expected future schema rejection")
	}
}
