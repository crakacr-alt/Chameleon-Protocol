package core

import (
	"net"
	"testing"
)

func TestTransportWithoutSharedSecretDoesNotPanic(t *testing.T) {
	t.Parallel()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	transport, err := NewTransport(clientConn, Config{Profile: ProfileWebRTC})
	if err != nil {
		t.Fatalf("NewTransport returned error: %v", err)
	}

	go func() {
		buf := make([]byte, 4096)
		_, _ = serverConn.Read(buf)
	}()

	if err := transport.Send([]byte("hello-chameleon")); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
}
