package crypto

import (
	"encoding/base64"
	"fmt"
	"crypto/ed25519"
)

// RekeyMessage is the signed payload sent between peers to initiate a key swap.
type RekeyMessage struct {
	EpochID string `json:"epoch_id"`
	KeyInfo string `json:"key_info"` // optional info
	Sig     string `json:"sig"`
}

// CreateRekeyMessage creates a RekeyMessage signed by the local identity.
func CreateRekeyMessage(km *KeyManager, epochID []byte, keyInfo string) (*RekeyMessage, error) {
	if km == nil {
		return nil, fmt.Errorf("key manager required")
	}
	payload := append(epochID, []byte(keyInfo)...)
	sig, err := km.Sign(payload)
	if err != nil {
		return nil, fmt.Errorf("sign rekey payload: %w", err)
	}
	return &RekeyMessage{EpochID: base64.StdEncoding.EncodeToString(epochID), KeyInfo: keyInfo, Sig: base64.StdEncoding.EncodeToString(sig)}, nil
}

// VerifyRekeyMessage verifies a rekey message using peer's public key.
func VerifyRekeyMessage(msg *RekeyMessage, peerPub []byte) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}
	epochID, err := base64.StdEncoding.DecodeString(msg.EpochID)
	if err != nil {
		return nil, fmt.Errorf("decode epoch id: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(msg.Sig)
	if err != nil {
		return nil, fmt.Errorf("decode sig: %w", err)
	}
	payload := append(epochID, []byte(msg.KeyInfo)...)
	if !ed25519.Verify(ed25519.PublicKey(peerPub), payload, sig) {
		return nil, fmt.Errorf("rekey signature verification failed")
	}
	return epochID, nil
}
