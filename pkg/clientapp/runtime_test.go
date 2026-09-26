package clientapp

import (
	"testing"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
)

func TestWithoutDirect(t *testing.T) {
	in := []carrier.Candidate{
		{Name: "direct", Kind: carrier.KindDirect},
		{Name: "quic", Kind: carrier.KindChameleonQUIC},
	}
	out := withoutDirect(in)
	if len(out) != 1 || out[0].Kind != carrier.KindChameleonQUIC {
		t.Fatalf("unexpected candidates: %+v", out)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	cfg := clientconfig.Default()
	cfg.Mode = clientconfig.ModeProxy
	if _, err := New(cfg); err == nil {
		t.Fatal("expected proxy config without server to fail")
	}
}

func TestMatchBypass(t *testing.T) {
	rules := []string{"localhost", "10.0.0.0/8", ".lan.example"}
	cases := map[string]bool{
		"127.0.0.1:80":          true,
		"10.2.3.4:443":          true,
		"router.lan.example:80": true,
		"lan.example:80":        true,
		"example.com:443":       false,
	}
	for destination, want := range cases {
		if got := MatchBypass(destination, rules); got != want {
			t.Fatalf("%s: want %t got %t", destination, want, got)
		}
	}
}
