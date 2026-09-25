package tunnel

import (
	"bytes"
	"testing"
	"time"
)

func TestClientHelloRoundTrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	raw, nonce, err := buildClientHello("secret", "example.com:443", now)
	if err != nil {
		t.Fatal(err)
	}

	hello, err := readClientHello(bytes.NewReader(raw), "secret", now.Add(10*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if hello.Destination != "example.com:443" {
		t.Fatalf("unexpected destination %q", hello.Destination)
	}
	if hello.Nonce != nonce {
		t.Fatal("nonce changed")
	}
}

func TestClientHelloRejectsWrongPSK(t *testing.T) {
	now := time.Now()
	raw, _, err := buildClientHello("secret", "example.com:443", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readClientHello(bytes.NewReader(raw), "wrong", now, time.Minute); err == nil {
		t.Fatal("expected authentication error")
	}
}

func TestClientHelloRejectsExpiredTimestamp(t *testing.T) {
	now := time.Now()
	raw, _, err := buildClientHello("secret", "example.com:443", now.Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readClientHello(bytes.NewReader(raw), "secret", now, time.Minute); err == nil {
		t.Fatal("expected timestamp error")
	}
}
