package core

import (
	"net"
	"testing"

	chameleoncrypto "github.com/Hack2p/chameleon/pkg/crypto"
)

func TestTransportFrameRoundTripWithEncryption(t *testing.T) {
	t.Parallel()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	transport, err := NewTransport(clientConn, Config{
		Profile:      ProfileWebRTC,
		SharedSecret: "shared-secret",
	})
	if err != nil {
		t.Fatalf("NewTransport returned error: %v", err)
	}

	gotFrame := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := serverConn.Read(buf)
		if err != nil {
			t.Errorf("serverConn.Read returned error: %v", err)
			return
		}
		gotFrame <- append([]byte(nil), buf[:n]...)
	}()

	if err := transport.Send([]byte("hello-chameleon")); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	frame := <-gotFrame
	if len(frame) < 12 {
		t.Fatalf("frame too short: %d bytes", len(frame))
	}

	decoded, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFrame returned error: %v", err)
	}

	cipher, err := chameleoncrypto.NewCipher("shared-secret")
	if err != nil {
		t.Fatalf("NewCipher returned error: %v", err)
	}

	opened, err := cipher.Open(decoded)
	if err != nil {
		t.Fatalf("cipher.Open returned error: %v", err)
	}

	if string(opened) != "hello-chameleon" {
		t.Fatalf("unexpected plaintext: %q", string(opened))
	}
}

func TestTransportUsesConfiguredProfileWhenNoAdaptiveSync(t *testing.T) {
	t.Parallel()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	transport, err := NewTransport(clientConn, Config{Profile: ProfileHTTP3})
	if err != nil {
		t.Fatalf("NewTransport returned error: %v", err)
	}

	if got := transport.profileName(); got != ProfileHTTP3 {
		t.Fatalf("unexpected profile name: %s; want %s", got, ProfileHTTP3)
	}

	_ = serverConn
}
