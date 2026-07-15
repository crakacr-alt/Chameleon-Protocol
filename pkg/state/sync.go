package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// Sync keeps a shared-secret epoch state for a profile rotation.
type Sync struct {
	PSK         string
	EpochPeriod time.Duration
}

// NewSync creates a synchronized profile coordinator over a shared secret.
func NewSync(psk string) *Sync {
	return &Sync{PSK: psk, EpochPeriod: time.Minute}
}

// ProfileAt returns the deterministic profile name for a given timestamp.
func (s *Sync) ProfileAt(ts time.Time) string {
	if s == nil || s.PSK == "" {
		return "webrtc"
	}

	period := s.EpochPeriod
	if period <= 0 {
		period = time.Minute
	}

	epoch := ts.UnixNano() / int64(period)
	if epoch < 0 {
		epoch = 0
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(epoch))

	mac := hmac.New(sha256.New, []byte(s.PSK))
	_, _ = mac.Write(buf)
	sum := mac.Sum(nil)

	profileOrder := []string{"webrtc", "http3", "gaming"}
	start := int(sum[0]) % len(profileOrder)
	index := (int(epoch) + start) % len(profileOrder)

	return profileOrder[index]
}
