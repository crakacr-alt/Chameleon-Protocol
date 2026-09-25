package crypto

import (
	"bytes"
	"testing"
)

func TestAuthenticatedHandshakesDeriveSameSessionKey(t *testing.T) {
	client, err := NewAuthHandshake()
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewAuthHandshake()
	if err != nil {
		t.Fatal(err)
	}

	clientSig, err := client.SignX25519()
	if err != nil {
		t.Fatal(err)
	}
	serverSig, err := server.SignX25519()
	if err != nil {
		t.Fatal(err)
	}

	clientShared, err := client.DeriveSharedSecret(server.X25519Public(), server.Ed25519Public(), serverSig)
	if err != nil {
		t.Fatal(err)
	}
	serverShared, err := server.DeriveSharedSecret(client.X25519Public(), client.Ed25519Public(), clientSig)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(clientShared, serverShared) {
		t.Fatal("X25519 shared secrets differ")
	}

	context := SessionContext(client.X25519Public(), server.X25519Public())
	clientKey, err := DeriveSessionKey(clientShared, context, 32)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := DeriveSessionKey(serverShared, context, 32)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(clientKey, serverKey) {
		t.Fatal("derived session keys differ")
	}
}
