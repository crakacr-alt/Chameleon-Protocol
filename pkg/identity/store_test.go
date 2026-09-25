package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterOrVerifyPinsFirstKey(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "identities.json"))
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.RegisterOrVerify("peer", "key-a")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first registration must create a pin")
	}

	created, err = store.RegisterOrVerify("peer", "key-a")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("same key must verify without creating a new pin")
	}

	if _, err := store.RegisterOrVerify("peer", "key-b"); err == nil {
		t.Fatal("different key for pinned identity must be rejected")
	}
}

func TestStorePathIsRuntimeOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identities.json")
	if err := os.WriteFile(path, []byte(`{"path":"/tmp/redirected.json","id_map":{"peer":"key-a"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Path != path {
		t.Fatalf("persisted JSON overrode runtime path: got %q want %q", store.Path, path)
	}
	if err := store.Register("peer-2", "key-b"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"path"`) {
		t.Fatalf("runtime path must not be persisted: %s", data)
	}
}
