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

func FuzzDecodeFrameDoesNotPanic(f *testing.F) {
	// Несколько нормальных и сломанных примеров дают fuzz-тесту стартовые точки.
	f.Add([]byte("CHLM"))
	f.Add([]byte("not-a-frame"))

	valid, err := EncodeFrame(ProfileWebRTC, []byte("hello-padding"), 5)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Для случайного входа допустима ошибка. Главное — parser не должен panic.
		_, _ = DecodeFrame(data)
	})
}
