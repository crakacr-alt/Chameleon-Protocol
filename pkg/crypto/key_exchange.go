package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// KeyExchange implements a minimal X25519-based handshake for deriving
// a shared secret between peers without introducing protocol bloat.
type KeyExchange struct {
	private []byte
	public  []byte
}

// NewKeyExchange creates a fresh ephemeral X25519 key pair.
func NewKeyExchange() (*KeyExchange, error) {
	private := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(private); err != nil {
		return nil, fmt.Errorf("generate private scalar: %w", err)
	}

	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}

	return &KeyExchange{
		private: private,
		public:  public,
	}, nil
}

// PublicKey returns the peer-visible portion of the key exchange.
func (k *KeyExchange) PublicKey() []byte {
	if k == nil {
		return nil
	}
	return k.public
}

// SharedSecret derives the symmetric session secret from the remote public key.
func (k *KeyExchange) SharedSecret(remote []byte) ([]byte, error) {
	if k == nil || len(k.private) == 0 {
		return nil, fmt.Errorf("key exchange is not initialized")
	}
	if len(remote) == 0 {
		return nil, fmt.Errorf("remote public key must not be empty")
	}

	secret, err := curve25519.X25519(k.private, remote)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}

	return secret, nil
}
