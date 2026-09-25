package core

import (
	"net"
	"testing"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/morph"
)

func TestTransportEntropyBudgetIsCumulative(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	transport, err := NewTransport(clientConn, Config{
		Profile: ProfileWebRTC,
		Padding: morph.PaddingConfig{
			MinPad:        10,
			MaxPad:        10,
			EntropyBudget: 15,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		_, _ = serverConn.Read(buf)
	}()

	if err := transport.Send([]byte("first")); err != nil {
		t.Fatalf("first Send returned error: %v", err)
	}
	<-readDone

	if err := transport.Send([]byte("second")); err == nil {
		t.Fatal("expected second Send to exceed the remaining entropy budget")
	}
}
