package morph

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// JitterConfig models a bounded delay schedule for packet emission.
type JitterConfig struct {
	BaseDelay time.Duration
	MinJitter time.Duration
	MaxJitter time.Duration
}

// Validate ensures the jitter schedule is well-formed.
func (c JitterConfig) Validate() error {
	if c.BaseDelay < 0 {
		return fmt.Errorf("base delay must be non-negative")
	}
	if c.MinJitter < 0 {
		return fmt.Errorf("min jitter must be non-negative")
	}
	if c.MaxJitter < c.MinJitter {
		return fmt.Errorf("max jitter must be greater than or equal to min jitter")
	}
	return nil
}

// Next returns a randomized delay based on the configured jitter window.
func (c JitterConfig) Next() (time.Duration, error) {
	if err := c.Validate(); err != nil {
		return 0, err
	}

	if c.MaxJitter == 0 {
		return c.BaseDelay, nil
	}

	span := int64(c.MaxJitter - c.MinJitter)
	if span == 0 {
		return c.BaseDelay + c.MinJitter, nil
	}

	offset, err := rand.Int(rand.Reader, big.NewInt(span+1))
	if err != nil {
		return 0, fmt.Errorf("generate jitter: %w", err)
	}

	return c.BaseDelay + c.MinJitter + time.Duration(offset.Int64()), nil
}
