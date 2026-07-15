package crypto

import "testing"

func TestHandshakeDerivesSameSharedSecret(t *testing.T) {
	t.Parallel()

	alice, err := NewHandshake()
	if err != nil {
		t.Fatalf("NewHandshake returned error: %v", err)
	}
	bob, err := NewHandshake()
	if err != nil {
		t.Fatalf("NewHandshake returned error: %v", err)
	}

	alicePub, err := alice.PublicKey()
	if err != nil {
		t.Fatalf("alice.PublicKey returned error: %v", err)
	}
	bobPub, err := bob.PublicKey()
	if err != nil {
		t.Fatalf("bob.PublicKey returned error: %v", err)
	}

	aliceSecret, err := alice.DeriveSharedSecret(bobPub)
	if err != nil {
		t.Fatalf("alice.DeriveSharedSecret returned error: %v", err)
	}
	bobSecret, err := bob.DeriveSharedSecret(alicePub)
	if err != nil {
		t.Fatalf("bob.DeriveSharedSecret returned error: %v", err)
	}

	if string(aliceSecret) != string(bobSecret) {
		t.Fatalf("shared secrets differ: %q vs %q", string(aliceSecret), string(bobSecret))
	}
}
