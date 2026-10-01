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
CHAMELEON_TCP_TRANSPORT=tls
CHAMELEON_UDP_MODE=auto
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
	if cfg.TCPTransport != "tls" || cfg.UDPMode != "auto" {
		t.Fatalf("transport preferences not imported: tcp=%q udp=%q", cfg.TCPTransport, cfg.UDPMode)
	}
}

func TestImportExplicitTLSDoesNotInventOtherTransports(t *testing.T) {
	cfg, err := ImportProfile(strings.NewReader(`
CHAMELEON_SERVER=relay.example:443
CHAMELEON_TLS_SERVER=relay.example:443
CHAMELEON_TUNNEL_PSK=0123456789abcdef
CHAMELEON_TLS_FINGERPRINT=aabbcc
CHAMELEON_TCP_TRANSPORT=tls
CHAMELEON_UDP_MODE=auto
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSServer != "relay.example:443" {
		t.Fatalf("TLS endpoint missing: %+v", cfg)
	}
	if cfg.TCPServer != "" || cfg.QUICServer != "" {
		t.Fatalf("explicit TLS-only profile invented unavailable transports: %+v", cfg)
	}
}

func TestImportLegacyProfileAddsTCPFallback(t *testing.T) {
	cfg, err := ImportProfile(strings.NewReader(`
CHAMELEON_SERVER=relay.example:9443
CHAMELEON_TUNNEL_PSK=0123456789abcdef
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TCPServer != "relay.example:9443" {
		t.Fatalf("legacy profile did not get TCP fallback: %+v", cfg)
	}
}

func TestImportAuthV2ProfileWithoutLegacyPSK(t *testing.T) {
	cfg, err := ImportProfile(strings.NewReader(`
CHAMELEON_SERVER=relay.example:443
CHAMELEON_TLS_SERVER=relay.example:443
CHAMELEON_CLIENT_ID=android-client-0001
CHAMELEON_CLIENT_SECRET=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
CHAMELEON_TLS_FINGERPRINT=aabbcc
CHAMELEON_TCP_TRANSPORT=tls
CHAMELEON_UDP_MODE=auto
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "android-client-0001" {
		t.Fatalf("client id not imported: %q", cfg.ClientID)
	}
	if len(cfg.ClientSecret) != 64 {
		t.Fatalf("client secret not imported: length=%d", len(cfg.ClientSecret))
	}
	if cfg.PSK != "" {
		t.Fatal("Auth v2 profile unexpectedly requires legacy PSK")
	}
	if cfg.SchemaVersion != SchemaVersion {
		t.Fatalf("profile schema not migrated: %d", cfg.SchemaVersion)
	}
}

func TestAuthV2RequiresIDAndSecretTogether(t *testing.T) {
	cfg := Default()
	cfg.TLSServer = "relay.example:443"
	cfg.ClientID = "android-client-0001"
	if err := cfg.Validate(); err == nil {
		t.Fatal("client_id without client_secret must be rejected")
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
