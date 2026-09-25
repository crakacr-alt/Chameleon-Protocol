package crypto

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyManagerCreateLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.key")

	km, err := NewKeyManager(path)
	if err != nil {
		t.Fatalf("new key manager: %v", err)
	}
	if len(km.Public()) == 0 {
		t.Fatal("expected public key")
	}

	km2, err := NewKeyManager(path)
	if err != nil {
		t.Fatalf("load key manager: %v", err)
	}
	if string(km.Public()) != string(km2.Public()) {
		t.Fatal("public key changed after reload")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestKeyManagerRejectsCorruptExistingIdentity(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "invalid-base64", data: []byte("not-base64")},
		{name: "wrong-size", data: []byte(base64.StdEncoding.EncodeToString([]byte("short")))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.key")
			if err := os.WriteFile(path, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := NewKeyManager(path); err == nil {
				t.Fatal("expected corrupt persistent identity to be rejected")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("corrupt identity was silently replaced")
			}
		})
	}
}
