package identity

import (
	"path/filepath"
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
