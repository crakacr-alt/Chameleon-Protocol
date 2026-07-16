package morph

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// PaddingConfig controls how much random payload padding is added to a packet.
type PaddingConfig struct {
	MinPad int
	MaxPad int
	// EntropyBudget limits how many padding bytes may be consumed per session
	// or per security context. If zero, no explicit limit is applied.
	EntropyBudget int64
}

// Validate ensures the padding configuration is safe and bounded.
func (c PaddingConfig) Validate() error {
	if c.MinPad < 0 {
		return fmt.Errorf("min padding must be non-negative")
	}
	if c.MaxPad < c.MinPad {
		return fmt.Errorf("max padding must be greater than or equal to min padding")
	}
	return nil
}

// TargetSize returns a randomized packet size based on the desired padding window.
func (c PaddingConfig) TargetSize(payloadLen int) (int, error) {
	if payloadLen < 0 {
		return 0, fmt.Errorf("payload length must be non-negative")
	}
	if err := c.Validate(); err != nil {
		return 0, err
	}

	paddingSpan := c.MaxPad - c.MinPad
	if paddingSpan == 0 {
		return payloadLen + c.MinPad, nil
	}

	rangeMax := big.NewInt(int64(paddingSpan + 1))
	offset, err := rand.Int(rand.Reader, rangeMax)
	if err != nil {
		return 0, fmt.Errorf("randomize target size: %w", err)
	}

	return payloadLen + c.MinPad + int(offset.Int64()), nil
}

// Normalize pads an outbound packet up to a target byte length.
func (c PaddingConfig) Normalize(payload []byte, targetSize int) ([]byte, error) {
	if targetSize < len(payload) {
		return nil, fmt.Errorf("target size %d is smaller than payload size %d", targetSize, len(payload))
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}

	normalized := make([]byte, 0, targetSize)
	normalized = append(normalized, payload...)

	paddingLen := targetSize - len(payload)
	if paddingLen == 0 {
		return normalized, nil
	}

	padding := make([]byte, paddingLen)
	if _, err := rand.Read(padding); err != nil {
		return nil, fmt.Errorf("generate padding: %w", err)
	}

	// If an entropy budget is configured, ensure we do not exceed it.
	if c.EntropyBudget > 0 {
		// Note: enforcement should be done by the caller who tracks session budget.
		// Here we conservatively check per-packet padding against the configured budget.
		if int64(paddingLen) > c.EntropyBudget {
			return nil, fmt.Errorf("padding %d exceeds entropy budget %d", paddingLen, c.EntropyBudget)
		}
	}

	return append(normalized, padding...), nil
}
