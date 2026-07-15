package core

import "testing"

func TestEncodeDecodeFrameRoundTrip(t *testing.T) {
	t.Parallel()

	payload := []byte("hello-chameleon")
	normalized := append(payload, []byte("padding-data")...)
	frame, err := EncodeFrame(ProfileWebRTC, normalized, len(payload))
	if err != nil {
		t.Fatalf("EncodeFrame returned error: %v", err)
	}

	decoded, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFrame returned error: %v", err)
	}

	if string(decoded) != string(payload) {
		t.Fatalf("roundtrip mismatch: got %q want %q", string(decoded), string(payload))
	}
}
