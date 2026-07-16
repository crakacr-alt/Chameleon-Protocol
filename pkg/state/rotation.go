// NOTE: rotation.go was replaced by pkg/state/sync.go earlier; keep only one Sync implementation.

package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// EpochID returns a compact epoch identifier for key derivation using Sync.PSK and EpochPeriod.
func (s *Sync) EpochID(ts time.Time) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("sync is nil")
	}
	period := s.EpochPeriod
	if period <= 0 {
		period = time.Minute
	}
	epoch := ts.UnixNano() / int64(period)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(epoch))
	mac := hmac.New(sha256.New, []byte(s.PSK))
	if _, err := mac.Write(buf); err != nil {
		return nil, fmt.Errorf("compute epoch id: %w", err)
	}
	return mac.Sum(nil), nil
}
