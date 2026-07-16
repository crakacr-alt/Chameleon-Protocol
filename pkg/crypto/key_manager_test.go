package crypto

import (
	"os"
	"testing"
)

func TestKeyManager_CreateLoad(t *testing.T) {
	path := "test_server.key"
	defer os.Remove(path)

	km, err := NewKeyManager(path)
	if err != nil {
		t.Fatalf("new key manager: %v", err)
	}
	if len(km.Public()) == 0 {
		t.Fatalf("expected public key")
	}

	// load again
	km2, err := NewKeyManager(path)
	if err != nil {
		t.Fatalf("load key manager: %v", err)
	}
	if string(km.Public()) != string(km2.Public()) {
		t.Fatalf("public mismatch")
	}
}
