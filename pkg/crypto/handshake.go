package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
)

// Handshake provides a minimal session-bootstrap handshake over X25519.
type Handshake struct {
	keyExchange *KeyExchange
}

// NewHandshake creates a fresh X25519 ephemeral key exchange for a session.
func NewHandshake() (*Handshake, error) {
	keyExchange, err := NewKeyExchange()
	if err != nil {
		return nil, fmt.Errorf("new key exchange: %w", err)
	}

	return &Handshake{keyExchange: keyExchange}, nil
}

// PublicKey returns the peer-visible key bytes for transport.
func (h *Handshake) PublicKey() ([]byte, error) {
	if h == nil || h.keyExchange == nil || h.keyExchange.public == nil {
		return nil, fmt.Errorf("handshake is not initialized")
	}

	return h.keyExchange.public.Bytes(), nil
}

// DeriveSharedSecret builds the shared session secret from the remote public key.
func (h *Handshake) DeriveSharedSecret(remotePublic []byte) ([]byte, error) {
	if h == nil || h.keyExchange == nil || h.keyExchange.private == nil {
		return nil, fmt.Errorf("handshake is not initialized")
	}
	if len(remotePublic) == 0 {
		return nil, fmt.Errorf("remote public key must not be empty")
	}

	curve := ecdh.X25519()
	remote, err := curve.NewPublicKey(remotePublic)
	if err != nil {
		return nil, fmt.Errorf("decode remote public key: %w", err)
	}

	secret, err := h.keyExchange.private.ECDH(remote)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}

	return secret, nil
}

// GenerateNonce creates a unique per-session nonce using crypto/rand.
func GenerateNonce(size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("nonce size must be positive")
	}

	nonce := make([]byte, size)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return nonce, nil
}
