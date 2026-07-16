package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
)

// AuthHandshake combines an ephemeral X25519 key exchange with an
// Ed25519 identity keypair to provide a simple authenticated handshake.
type AuthHandshake struct {
	ke       *KeyExchange
	edPub    ed25519.PublicKey
	edPriv   ed25519.PrivateKey
}

// NewAuthHandshake generates a fresh X25519 ephemeral key pair and
// a long-lived Ed25519 identity for signing the handshake.
func NewAuthHandshake() (*AuthHandshake, error) {
	ke, err := NewKeyExchange()
	if err != nil {
		return nil, fmt.Errorf("new key exchange: %w", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	return &AuthHandshake{ke: ke, edPub: pub, edPriv: priv}, nil
}

// NewAuthHandshakeWithKeyManager creates an AuthHandshake using a persistent
// Ed25519 identity from KeyManager and a fresh ephemeral X25519 key exchange.
func NewAuthHandshakeWithKeyManager(km *KeyManager) (*AuthHandshake, error) {
	if km == nil {
		return nil, fmt.Errorf("key manager required")
	}
	ke, err := NewKeyExchange()
	if err != nil {
		return nil, fmt.Errorf("new key exchange: %w", err)
	}
	// KeyManager holds private key internally; we can access it since same package
	return &AuthHandshake{ke: ke, edPub: km.pub, edPriv: km.priv}, nil
}

// X25519Public returns the ephemeral X25519 public key for the handshake.
func (a *AuthHandshake) X25519Public() []byte {
	if a == nil || a.ke == nil {
		return nil
	}
	return append([]byte(nil), a.ke.PublicKey()...)
}

// Ed25519Public returns the identity public key bytes.
func (a *AuthHandshake) Ed25519Public() []byte {
	if a == nil || len(a.edPub) == 0 {
		return nil
	}
	return append([]byte(nil), a.edPub...)
}

// SignX25519 signs the X25519 public key with the Ed25519 identity key.
func (a *AuthHandshake) SignX25519() ([]byte, error) {
	if a == nil || a.ke == nil || len(a.edPriv) == 0 {
		return nil, fmt.Errorf("handshake not initialized")
	}
	msg := a.ke.PublicKey()
	sig := ed25519.Sign(a.edPriv, msg)
	return append([]byte(nil), sig...), nil
}

// VerifySignedPublic verifies a peer-signed X25519 public using their Ed25519 key.
func VerifySignedPublic(peerXpub, peerEdPub, sig []byte) error {
	if len(peerXpub) == 0 {
		return fmt.Errorf("peer x25519 public empty")
	}
	if len(peerEdPub) == 0 {
		return fmt.Errorf("peer ed25519 public empty")
	}
	if len(sig) == 0 {
		return fmt.Errorf("signature empty")
	}
	if !ed25519.Verify(ed25519.PublicKey(peerEdPub), peerXpub, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// DeriveSharedSecret verifies the peer signature and derives the X25519 shared secret.
func (a *AuthHandshake) DeriveSharedSecret(peerXpub, peerEdPub, sig []byte) ([]byte, error) {
	if err := VerifySignedPublic(peerXpub, peerEdPub, sig); err != nil {
		return nil, fmt.Errorf("verify peer signed public: %w", err)
	}
	if a == nil || a.ke == nil {
		return nil, fmt.Errorf("handshake not initialized")
	}
	return a.ke.SharedSecret(peerXpub)
}
