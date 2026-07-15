package crypto

import "testing"

func TestCipherRoundTrip(t *testing.T) {
	t.Parallel()

	cipher, err := NewCipher("shared-secret")
	if err != nil {
		t.Fatalf("NewCipher returned error: %v", err)
	}

	plaintext := []byte("hello-chameleon")
	sealed, err := cipher.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal returned error: %v", err)
	}

	opened, err := cipher.Open(sealed)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	if string(opened) != string(plaintext) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", string(opened), string(plaintext))
	}
}

func TestKeyExchangeProducesSharedSecret(t *testing.T) {
	t.Parallel()

	alice, err := NewKeyExchange()
	if err != nil {
		t.Fatalf("NewKeyExchange returned error: %v", err)
	}
	bob, err := NewKeyExchange()
	if err != nil {
		t.Fatalf("NewKeyExchange returned error: %v", err)
	}

	aliceSecret, err := alice.SharedSecret(bob.PublicKey())
	if err != nil {
		t.Fatalf("alice.SharedSecret returned error: %v", err)
	}
	bobSecret, err := bob.SharedSecret(alice.PublicKey())
	if err != nil {
		t.Fatalf("bob.SharedSecret returned error: %v", err)
	}

	if string(aliceSecret) != string(bobSecret) {
		t.Fatalf("shared secrets differ: %q vs %q", string(aliceSecret), string(bobSecret))
	}
}
