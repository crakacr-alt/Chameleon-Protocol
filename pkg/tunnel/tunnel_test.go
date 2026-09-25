package tunnel

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestAuthenticatedTunnelEndToEnd(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	upstreamServer, upstreamEcho := net.Pipe()

	go func() {
		_, _ = io.Copy(upstreamEcho, upstreamEcho)
		_ = upstreamEcho.Close()
	}()

	server, err := NewServer(ServerConfig{
		PSK:              "secret",
		HandshakeTimeout: time.Second,
		DialTimeout:      time.Second,
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

	secure, err := clientHandshake(clientSide, "example.com:443", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := secure.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 5)
	if _, err := io.ReadFull(secure, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "hello" {
		t.Fatalf("unexpected reply %q", reply)
	}

	_ = secure.Close()
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler did not stop")
	}
}

func TestReplayNonceRejected(t *testing.T) {
	server, err := NewServer(ServerConfig{PSK: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	var nonce [nonceSize]byte
	nonce[0] = 1
	now := time.Now()
	if !server.acceptNonce(nonce, now) {
		t.Fatal("first nonce should be accepted")
	}
	if server.acceptNonce(nonce, now.Add(time.Second)) {
		t.Fatal("replayed nonce should be rejected")
	}
}
