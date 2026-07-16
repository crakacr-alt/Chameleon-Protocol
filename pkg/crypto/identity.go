package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// WrapIdentityPayload creates a compact identity envelope with auth tag.
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

	tag := hmac.New(sha256.New, []byte(psk))
	if _, err := tag.Write(body); err != nil {
		return nil, fmt.Errorf("compute identity tag: %w", err)
	}

	wrapped := append([]byte(nil), body...)
	wrapped = append(wrapped, []byte(".")...)
	wrapped = append(wrapped, tag.Sum(nil)...)

	return []byte(base64.StdEncoding.EncodeToString(wrapped)), nil
}

// ParseIdentityPayload verifies an identity envelope and returns the decoded identity and payload.
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

	parts := len(decoded)
	if parts < sha256.Size {
		return "", nil, fmt.Errorf("identity envelope too short")
	}

	body := decoded[:parts-sha256.Size]
	tag := decoded[parts-sha256.Size:]

	want := hmac.New(sha256.New, []byte(psk))
	if _, err := want.Write(body); err != nil {
		return "", nil, fmt.Errorf("compute envelope tag: %w", err)
	}

	if !hmac.Equal(want.Sum(nil), tag) {
		return "", nil, fmt.Errorf("identity envelope authentication failed")
	}

	if idx := bytesIndex(body, '.'); idx >= 0 {
		body = body[idx+1:]
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

func bytesIndex(data []byte, b byte) int {
	for i := range data {
		if data[i] == b {
			return i
		}
	}
	return -1
}
