package tunnel

import (
	"bytes"
	"io"
	"net"
	"testing"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
)

func TestSecureConnRoundTripLargePayload(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	cipherA, err := chcrypto.NewCipher("secret")
	if err != nil {
		t.Fatal(err)
	}
	cipherB, err := chcrypto.NewCipher("secret")
	if err != nil {
		t.Fatal(err)
	}

	left := newSecureConn(a, cipherA)
	right := newSecureConn(b, cipherB)

	payload := bytes.Repeat([]byte("abc123"), 20000)
	errCh := make(chan error, 1)
	go func() {
		_, writeErr := left.Write(payload)
		errCh <- writeErr
	}()

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(right, got); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("payload changed after secure framing")
	}
}
