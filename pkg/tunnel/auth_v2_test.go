package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

const (
	testClientID     = "android-test-client-0001"
	testClientSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func testV2Registry(t *testing.T) *ClientRegistry {
	t.Helper()
	registry, err := NewClientRegistry([]ClientCredential{{
		ID:      testClientID,
		Name:    "Android test",
		Secret:  testClientSecret,
		Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestAuthV2EndToEnd(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	upstreamServer, upstreamEcho := net.Pipe()

	go func() {
		_, _ = io.Copy(upstreamEcho, upstreamEcho)
		_ = upstreamEcho.Close()
	}()

	server, err := NewServer(ServerConfig{
		Clients:                  testV2Registry(t),
		HandshakeTimeout:         time.Second,
		DialTimeout:              time.Second,
		AllowPrivateDestinations: true,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return upstreamServer, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.HandleConn(context.Background(), serverSide)
	}()

	secure, err := clientHandshakeAuth(clientSide, "example.com:443", ClientAuth{
		ClientID:     testClientID,
		ClientSecret: testClientSecret,
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("auth-v2-round-trip")
	if _, err := secure.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(secure, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("unexpected payload %q", got)
	}
	_ = secure.Close()

	select {
	case err := <-serverDone:
		if err != nil && !isClosedError(err) && !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}

func TestAuthV2WrongSecretRejected(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	server, err := NewServer(ServerConfig{
		Clients:          testV2Registry(t),
		HandshakeTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.HandleConn(context.Background(), serverSide)
	}()

	_, err = clientHandshakeAuth(clientSide, probeSessionDestination, ClientAuth{
		ClientID:     testClientID,
		ClientSecret: strings.Repeat("f", 64),
	}, time.Second)
	if err == nil {
		t.Fatal("wrong Auth v2 secret must be rejected")
	}

	select {
	case serverErr := <-serverDone:
		if serverErr == nil || !strings.Contains(serverErr.Error(), "invalid tunnel authentication") {
			t.Fatalf("unexpected server error: %v", serverErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not reject wrong secret")
	}
}

func TestAuthV2DisabledAndExpiredClientsRejected(t *testing.T) {
	expired := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	registry, err := NewClientRegistry([]ClientCredential{
		{ID: "disabled-client-01", Secret: strings.Repeat("a", 64), Enabled: false},
		{ID: "expired-client-001", Secret: strings.Repeat("b", 64), Enabled: true, ExpiresAt: expired},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Resolve("disabled-client-01", time.Now()); ok {
		t.Fatal("disabled client must not resolve")
	}
	if _, ok := registry.Resolve("expired-client-001", time.Now()); ok {
		t.Fatal("expired client must not resolve")
	}
}

func TestServerKeepsLegacyPSKFallbackWithAuthV2Enabled(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	server, err := NewServer(ServerConfig{
		PSK:              "legacy-secret",
		Clients:          testV2Registry(t),
		HandshakeTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.HandleConn(context.Background(), serverSide)
	}()

	secure, err := clientHandshake(clientSide, probeSessionDestination, "legacy-secret", time.Second)
	if err != nil {
		t.Fatalf("legacy PSK fallback failed: %v", err)
	}
	_ = secure.Close()

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("legacy probe did not finish")
	}
}
