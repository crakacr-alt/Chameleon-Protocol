package clientconfig

import (
	"strings"
	"testing"
)

func TestPrepareBridgeTLSWithAlternateEndpoint(t *testing.T) {
	cfg := Default()
	cfg.Mode = ModeSmart
	cfg.TLSServer = "blocked.example:9443"
	cfg.TCPServer = "blocked.example:9443"
	cfg.QUICServer = "blocked.example:9443"
	cfg.ClientID = "android-client-0001"
	cfg.ClientSecret = strings.Repeat("a", 48)
	cfg.TLSFingerprint = strings.Repeat("b", 64)

	got, err := PrepareBridge(cfg, "100.101.102.103:9443")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeProxy || got.TCPTransport != "tls" || got.UDPMode != "auto" {
		t.Fatalf("bridge did not select fail-closed TLS proxy: %+v", got)
	}
	if got.TLSServer != "100.101.102.103:9443" || got.TCPServer != "" || got.QUICServer != "" {
		t.Fatalf("incorrect TCP/QUIC endpoints: %+v", got)
	}
	if got.ClientID != cfg.ClientID || got.ClientSecret != cfg.ClientSecret || got.TLSFingerprint != cfg.TLSFingerprint {
		t.Fatal("bridge changed authentication or TLS pin")
	}
	if cfg.TLSServer != "blocked.example:9443" {
		t.Fatal("bridge mutated the original profile")
	}
}

func TestPrepareBridgeKeepsOriginalEndpointWhenNoOverride(t *testing.T) {
	cfg := Default()
	cfg.TLSServer = "existing.example:443"
	cfg.PSK = "0123456789abcdef"
	got, err := PrepareBridge(cfg, " ")
	if err != nil {
		t.Fatal(err)
	}
	if got.TLSServer != cfg.TLSServer || got.Mode != ModeProxy {
		t.Fatalf("original TLS endpoint not preserved: %+v", got)
	}
}

func TestPrepareBridgeAcceptsBracketedIPv6(t *testing.T) {
	cfg := Default()
	cfg.TLSServer = "old.example:443"
	cfg.PSK = "0123456789abcdef"
	got, err := PrepareBridge(cfg, "[2001:db8::1]:9443")
	if err != nil {
		t.Fatal(err)
	}
	if got.TLSServer != "[2001:db8::1]:9443" {
		t.Fatalf("incorrect IPv6 endpoint: %q", got.TLSServer)
	}
}

func TestPrepareBridgeTCPOnly(t *testing.T) {
	cfg := Default()
	cfg.TCPServer = "original.example:9443"
	cfg.PSK = "0123456789abcdef"
	got, err := PrepareBridge(cfg, "100.99.88.77:9443")
	if err != nil {
		t.Fatal(err)
	}
	if got.TCPServer != "100.99.88.77:9443" || got.TCPTransport != "tcp" || got.QUICServer != "" {
		t.Fatalf("TCP-only bridge incorrect: %+v", got)
	}
}

func TestPrepareBridgeRejectsInvalidEndpointAndQUICOnly(t *testing.T) {
	cfg := Default()
	cfg.TLSServer = "original.example:443"
	cfg.PSK = "0123456789abcdef"
	for _, endpoint := range []string{"2001:db8::1:9443", "host", "host:0", "host:65536", ":443", "host:notaport"} {
		if _, err := PrepareBridge(cfg, endpoint); err == nil {
			t.Errorf("invalid Bridge endpoint accepted: %q", endpoint)
		}
	}
	cfg.TLSServer = ""
	cfg.QUICServer = "quic.example:443"
	if _, err := PrepareBridge(cfg, "100.99.88.77:9443"); err == nil {
		t.Fatal("QUIC-only profile must be rejected in TCP bridge mode")
	}
}
