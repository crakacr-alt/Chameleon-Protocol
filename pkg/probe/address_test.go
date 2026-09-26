package probe

import (
	"context"
	"net"
	"testing"
)

type staticResolver struct {
	addresses []net.IPAddr
	err       error
}

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addresses, r.err
}

func TestResolveAddressCandidatesPrefersIPv6ThenIPv4(t *testing.T) {
	got, err := ResolveAddressCandidates(
		context.Background(),
		"example.com:443",
		staticResolver{addresses: []net.IPAddr{
			{IP: net.ParseIP("192.0.2.10")},
			{IP: net.ParseIP("2001:db8::10")},
			{IP: net.ParseIP("192.0.2.11")},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want two families, got %+v", got)
	}
	if got[0].Family != FamilyIPv6 || got[0].Address != "[2001:db8::10]:443" {
		t.Fatalf("unexpected first candidate %+v", got[0])
	}
	if got[1].Family != FamilyIPv4 || got[1].Address != "192.0.2.10:443" {
		t.Fatalf("unexpected second candidate %+v", got[1])
	}
}

func TestResolveAddressCandidatesLiteral(t *testing.T) {
	got, err := ResolveAddressCandidates(context.Background(), "127.0.0.1:80", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Family != FamilyIPv4 {
		t.Fatalf("unexpected literal resolution %+v", got)
	}
}
