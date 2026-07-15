package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
)

// KeyExchange implements a minimal X25519-based handshake for deriving
// a shared secret between peers without introducing protocol bloat.
type KeyExchange struct {
	private *ecdh.PrivateKey
	public  *ecdh.PublicKey
}

// NewKeyExchange creates a fresh ephemeral X25519 key pair.
func NewKeyExchange() (*KeyExchange, error) {
	curve := ecdh.X25519()
	private, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}

	return &KeyExchange{
		private: private,
		public:  private.PublicKey(),
	}, nil
}

// PublicKey returns the peer-visible portion of the key exchange.
func (k *KeyExchange) PublicKey() *ecdh.PublicKey {
	if k == nil {
		return nil
	}
	return k.public
}

// SharedSecret derives the symmetric session secret from the remote public key.
func (k *KeyExchange) SharedSecret(remote *ecdh.PublicKey) ([]byte, error) {
	if k == nil || k.private == nil {
		return nil, fmt.Errorf("key exchange is not initialized")
	}
	if remote == nil {
		return nil, fmt.Errorf("remote public key must not be nil")
	}

	secret, err := k.private.ECDH(remote)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}

	return secret, nil
}
