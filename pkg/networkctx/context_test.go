package networkctx

import "testing"

func TestClassifyInterface(t *testing.T) {
	cases := map[string]LinkType{
		"eth0":        LinkEthernet,
		"enp3s0":      LinkEthernet,
		"wlan0":       LinkWiFi,
		"wlp2s0":      LinkWiFi,
		"rmnet_data0": LinkMobile,
		"tailscale0":  LinkVirtual,
		"wg0":         LinkVirtual,
		"utun4":       LinkVirtual,
	}

	for name, want := range cases {
		if got := ClassifyInterface(name); got != want {
			t.Fatalf("%s: want %s, got %s", name, want, got)
		}
	}
}

func TestFingerprintStable(t *testing.T) {
	ctx := Context{Link: LinkWiFi, OS: "linux"}
	a := fingerprint(ctx, []string{"192.168.1.0/24", "2001:db8::/64"})
	b := fingerprint(ctx, []string{"192.168.1.0/24", "2001:db8::/64"})
	if a != b {
		t.Fatalf("fingerprint is not stable: %q != %q", a, b)
	}
	if a == "" || a == "unknown" {
		t.Fatalf("unexpected fingerprint %q", a)
	}
}
