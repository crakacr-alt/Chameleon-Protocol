package morph

import "testing"

func TestPaddingConfigTargetSizeWithinBounds(t *testing.T) {
	t.Parallel()

	cfg := PaddingConfig{MinPad: 16, MaxPad: 32}

	target, err := cfg.TargetSize(8)
	if err != nil {
		t.Fatalf("TargetSize returned error: %v", err)
	}

	if target < 8+16 || target > 8+32 {
		t.Fatalf("unexpected target size %d; want range [%d, %d]", target, 8+16, 8+32)
	}
}

func TestPaddingConfigNormalizePreservesPayload(t *testing.T) {
	t.Parallel()

	cfg := PaddingConfig{MinPad: 4, MaxPad: 4}
	payload := []byte("abc")

	normalized, err := cfg.Normalize(payload, 7)
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}

	if len(normalized) != 7 {
		t.Fatalf("normalized length = %d; want %d", len(normalized), 7)
	}
	if string(normalized[:3]) != string(payload) {
		t.Fatalf("payload was not preserved")
	}
}
