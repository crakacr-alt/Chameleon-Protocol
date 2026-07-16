package crypto

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

// DeriveKey uses HKDF-SHA256 to produce a key of given length from ikm and salt.
func DeriveKey(ikm, salt, info []byte, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	hk := hkdf.New(sha256.New, ikm, salt, info)
	out := make([]byte, length)
	if _, err := io.ReadFull(hk, out); err != nil {
		return nil, err
	}
	return out, nil
}
