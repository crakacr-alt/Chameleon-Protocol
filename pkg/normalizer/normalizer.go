package normalizer

import (
	"fmt"
	"time"

	"github.com/Hack2p/chameleon/pkg/core"
)

// Normalizer exposes a small interface for packet normalization so other
// packages can depend on a stable API without touching morph internals.
type Normalizer interface {
	NormalizePacket(payload []byte) ([]byte, time.Duration, error)
}

type wrapper struct{
	n *core.Normalizer
}

// NewNormalizer creates a wrapper around the existing core Normalizer.
func NewNormalizer(cfg core.Config) (Normalizer, error) {
	n, err := core.NewNormalizer(cfg)
	if err != nil {
		return nil, err
	}
	return &wrapper{n: n}, nil
}

func (w *wrapper) NormalizePacket(payload []byte) ([]byte, time.Duration, error) {
	if w == nil || w.n == nil {
		return nil, 0, fmtError("normalizer not initialized")
	}
	return w.n.NormalizePacket(payload)
}

// small helper to avoid importing fmt in many places
func fmtError(msg string) error { return fmt.Errorf(msg) }
