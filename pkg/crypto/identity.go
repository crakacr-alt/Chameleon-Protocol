package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// WrapIdentityPayload creates a compact identity envelope with an HMAC tag.
// Wire format after base64 decoding is: JSON body || 32-byte HMAC-SHA256 tag.
func WrapIdentityPayload(identity, psk string, payload []byte) ([]byte, error) {
	if identity == "" {
		return nil, fmt.Errorf("identity must not be empty")
	}
	if psk == "" {
		return nil, fmt.Errorf("psk must not be empty")
	}

	body, err := json.Marshal(struct {
		Identity string `json:"identity"`
		Payload  []byte `json:"payload"`
	}{
		Identity: identity,
		Payload:  payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal identity payload: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(psk))
	if _, err := mac.Write(body); err != nil {
		return nil, fmt.Errorf("compute identity tag: %w", err)
	}

	wrapped := make([]byte, 0, len(body)+sha256.Size)
	wrapped = append(wrapped, body...)
	wrapped = append(wrapped, mac.Sum(nil)...)

	return []byte(base64.StdEncoding.EncodeToString(wrapped)), nil
}

// ParseIdentityPayload verifies an identity envelope and returns the decoded identity and payload.
// It also accepts the short-lived legacy format JSON body || "." || tag.
func ParseIdentityPayload(encoded []byte, psk string) (string, []byte, error) {
	if len(encoded) == 0 {
		return "", nil, fmt.Errorf("encoded identity payload must not be empty")
	}
	if psk == "" {
		return "", nil, fmt.Errorf("psk must not be empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return "", nil, fmt.Errorf("decode envelope: %w", err)
	}
	if len(decoded) <= sha256.Size {
		return "", nil, fmt.Errorf("identity envelope too short")
	}

	body := decoded[:len(decoded)-sha256.Size]
	tag := decoded[len(decoded)-sha256.Size:]

	if !validIdentityTag(body, tag, psk) {
		if len(body) == 0 || body[len(body)-1] != '.' {
			return "", nil, fmt.Errorf("identity envelope authentication failed")
		}

		legacyBody := body[:len(body)-1]
		if !validIdentityTag(legacyBody, tag, psk) {
			return "", nil, fmt.Errorf("identity envelope authentication failed")
		}
		body = legacyBody
	}

	var envelope struct {
		Identity string `json:"identity"`
		Payload  []byte `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", nil, fmt.Errorf("unmarshal identity payload: %w", err)
	}
	if envelope.Identity == "" {
		return "", nil, fmt.Errorf("identity must not be empty")
	}

	return envelope.Identity, envelope.Payload, nil
}

func validIdentityTag(body, tag []byte, psk string) bool {
	mac := hmac.New(sha256.New, []byte(psk))
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), tag)
}
