package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/hkdf"
)

// KeyManager manages a persistent Ed25519 identity and provides
// HKDF-based key derivation utilities for epoch/key lifecycle.
type KeyManager struct {
	Path string
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// NewKeyManager loads or creates an Ed25519 identity saved at path.
func NewKeyManager(path string) (*KeyManager, error) {
	if path == "" {
		return nil, fmt.Errorf("key path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir key dir: %w", err)
	}

	km := &KeyManager{Path: path}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		// file contains base64-encoded private key
		raw, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return nil, fmt.Errorf("decode private key: %w", err)
		}
		if len(raw) != ed25519.PrivateKeySize && len(raw) != ed25519.PrivateKeySize+0 {
			// allow variable encoding lengths but validate minimum
			// fallthrough to generate new key if invalid
		} else {
			km.priv = ed25519.PrivateKey(raw)
			km.pub = km.priv.Public().(ed25519.PublicKey)
			return km, nil
		}
	}

	// generate new keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	km.priv = priv
	km.pub = pub

	if err := km.saveLocked(); err != nil {
		return nil, err
	}
	return km, nil
}

// Public returns the Ed25519 public key bytes.
func (k *KeyManager) Public() []byte {
	if k == nil || len(k.pub) == 0 {
		return nil
	}
	return append([]byte(nil), k.pub...)
}

// Sign signs a message with the identity private key.
func (k *KeyManager) Sign(msg []byte) ([]byte, error) {
	if k == nil || len(k.priv) == 0 {
		return nil, fmt.Errorf("key manager not initialized")
	}
	sig := ed25519.Sign(k.priv, msg)
	return append([]byte(nil), sig...), nil
}

// GetEpochKey derives a symmetric key for the provided epochID using HKDF-SHA256.
// length specifies desired output key length in bytes (e.g., 32 for AES-256).
func (k *KeyManager) GetEpochKey(epochID []byte, length int) ([]byte, error) {
	if k == nil || len(k.priv) == 0 {
		return nil, fmt.Errorf("key manager not initialized")
	}
	if len(epochID) == 0 {
		return nil, fmt.Errorf("epochID must not be empty")
	}
	if length <= 0 {
		return nil, fmt.Errorf("length must be positive")
	}

	// use the private key as HKDF input key material (IKM); epochID acts as salt
	ikm := k.priv
	hk := hkdf.New(sha256.New, ikm, epochID, []byte("chameleon epoch key"))
	out := make([]byte, length)
	if _, err := io.ReadFull(hk, out); err != nil {
		return nil, fmt.Errorf("derive epoch key: %w", err)
	}
	return out, nil
}

func (k *KeyManager) saveLocked() error {
	if k == nil || len(k.priv) == 0 {
		return fmt.Errorf("key manager not initialized")
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(k.priv))
	if err := os.WriteFile(k.Path, []byte(encoded), 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}
	return nil
}
