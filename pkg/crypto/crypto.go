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
