package core

import (
	"time"

	"github.com/Hack2p/chameleon/pkg/morph"
)

// BehaviorProfile selects the traffic template the wrapper should emulate.
type BehaviorProfile string

const (
	ProfileWebRTC BehaviorProfile = "webrtc"
	ProfileHTTP3  BehaviorProfile = "http3"
	ProfileGaming BehaviorProfile = "gaming"
)

// Defaults returns a safe traffic profile for research and reproduction.
func (p BehaviorProfile) Defaults() Config {
	switch p {
	case ProfileWebRTC:
		return Config{
			Padding: morph.PaddingConfig{MinPad: 32, MaxPad: 128},
			Jitter: morph.JitterConfig{
				BaseDelay: 1 * time.Millisecond,
				MinJitter: 0,
				MaxJitter: 4 * time.Millisecond,
			},
		}
	case ProfileHTTP3:
		return Config{
			Padding: morph.PaddingConfig{MinPad: 16, MaxPad: 96},
			Jitter: morph.JitterConfig{
				BaseDelay: 500 * time.Microsecond,
				MinJitter: 0,
				MaxJitter: 2 * time.Millisecond,
			},
		}
	case ProfileGaming:
		return Config{
			Padding: morph.PaddingConfig{MinPad: 64, MaxPad: 256},
			Jitter: morph.JitterConfig{
				BaseDelay: 2 * time.Millisecond,
				MinJitter: 0,
				MaxJitter: 10 * time.Millisecond,
			},
		}
	default:
		return Config{
			Padding: morph.PaddingConfig{MinPad: 8, MaxPad: 32},
			Jitter: morph.JitterConfig{
				BaseDelay: 1 * time.Millisecond,
				MinJitter: 0,
				MaxJitter: 4 * time.Millisecond,
			},
		}
	}
}
