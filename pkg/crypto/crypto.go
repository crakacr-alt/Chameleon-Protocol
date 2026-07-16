package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// Cipher provides a small AEAD wrapper for packet payloads.
type Cipher struct {
	block cipher.AEAD
}

// NewCipherFromKey creates an AEAD cipher from raw key material. If the
// provided key is not 16/24/32 bytes long, it is hashed with SHA-256 to 32 bytes.
func NewCipherFromKey(key []byte) (*Cipher, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		k := sha256.Sum256(key)
		key = k[:]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher from key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm from key: %w", err)
	}
	return &Cipher{block: aead}, nil
}

// NewCipher derives a symmetric key from a shared passphrase.
func NewCipher(passphrase string) (*Cipher, error) {
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	return &Cipher{block: aead}, nil
}

// Seal encrypts plaintext and returns nonce || ciphertext.
func (c *Cipher) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.block.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	sealed := c.block.Seal(nonce, nonce, plaintext, nil)
	return sealed, nil
}

// Open decrypts nonce || ciphertext and returns the original plaintext.
func (c *Cipher) Open(frame []byte) ([]byte, error) {
	nonceSize := c.block.NonceSize()
	if len(frame) < nonceSize {
		return nil, fmt.Errorf("frame too short")
	}

	nonce := frame[:nonceSize]
	ciphertext := frame[nonceSize:]
	plaintext, err := c.block.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("open frame: %w", err)
	}

	return plaintext, nil
}
