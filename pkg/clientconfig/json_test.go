package clientconfig

import (
	"testing"
)

func TestParseJSONUsesDefaults(t *testing.T) {
	cfg, err := ParseJSON([]byte(`{
  "schema_version": 1,
  "mode": "smart",
  "listen": "127.0.0.1:1080",
  "udp_mode": "auto",
  "psk": ""
}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:1080" {
		t.Fatalf("unexpected listen address %q", cfg.Listen)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	cfg := Default()
	data, err := JSON(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != cfg.Mode || got.UDPMode != cfg.UDPMode {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
