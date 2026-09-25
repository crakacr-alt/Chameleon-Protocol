package crypto

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// SessionContext binds a session key to the ordered client/server ephemeral keys.
func SessionContext(clientX25519Public, serverX25519Public []byte) []byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("chameleon/session/v1"))
	_, _ = hash.Write(clientX25519Public)
	_, _ = hash.Write(serverX25519Public)
	return hash.Sum(nil)
}

// DeriveSessionKey derives peer-shared key material from an X25519 shared secret.
func DeriveSessionKey(sharedSecret, context []byte, length int) ([]byte, error) {
	if len(sharedSecret) == 0 {
		return nil, fmt.Errorf("shared secret must not be empty")
	}
	if len(context) == 0 {
		return nil, fmt.Errorf("session context must not be empty")
	}
	if length <= 0 {
		return nil, fmt.Errorf("length must be positive")
	}

	reader := hkdf.New(sha256.New, sharedSecret, context, []byte("chameleon peer session key"))
	key := make([]byte, length)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive session key: %w", err)
	}
	return key, nil
}
