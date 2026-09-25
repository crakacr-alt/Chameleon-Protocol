package core

import (
	"net"
	"testing"

	chameleoncrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
)

func TestTransportConcurrentCipherUpdateAndSend(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	transport, err := NewTransport(clientConn, Config{Profile: ProfileWebRTC})
	if err != nil {
		t.Fatal(err)
	}

	first, err := chameleoncrypto.NewCipher("first-secret")
	if err != nil {
		t.Fatal(err)
	}
	second, err := chameleoncrypto.NewCipher("second-secret")
	if err != nil {
		t.Fatal(err)
	}
	transport.UpdateCipher(first)

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 4096)
		for i := 0; i < 100; i++ {
			if _, err := serverConn.Read(buf); err != nil {
				return
			}
		}
	}()

	sendDone := make(chan error, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if err := transport.Send([]byte("race-check")); err != nil {
				sendDone <- err
				return
			}
		}
		sendDone <- nil
	}()

	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			transport.UpdateCipher(first)
		} else {
			transport.UpdateCipher(second)
		}
	}

	if err := <-sendDone; err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	<-readerDone
}
