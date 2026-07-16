package crypto

import "testing"

func TestWrapAndParseIdentityPayload(t *testing.T) {
	t.Parallel()

	const identity = "alice-mobile"
	const psk = "research-secret"
	payload := []byte("hello-auth")

	wrapped, err := WrapIdentityPayload(identity, psk, payload)
	if err != nil {
		t.Fatalf("WrapIdentityPayload returned error: %v", err)
	}

	parsedIdentity, parsedPayload, err := ParseIdentityPayload(wrapped, psk)
	if err != nil {
		t.Fatalf("ParseIdentityPayload returned error: %v", err)
	}

	if parsedIdentity != identity {
		t.Fatalf("parsed identity mismatch: got %q, want %q", parsedIdentity, identity)
	}

	if string(parsedPayload) != string(payload) {
		t.Fatalf("parsed payload mismatch: got %q, want %q", string(parsedPayload), string(payload))
	}
}

func TestWrapIdentityPayloadRejectsWrongPSK(t *testing.T) {
	t.Parallel()

	wrapped, err := WrapIdentityPayload("alice-mobile", "research-secret", []byte("hello-auth"))
	if err != nil {
		t.Fatalf("WrapIdentityPayload returned error: %v", err)
	}

	if _, _, err := ParseIdentityPayload(wrapped, "wrong-secret"); err == nil {
		t.Fatal("ParseIdentityPayload accepted payload with wrong PSK")
	}
}
