package core

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Hack2p/chameleon/pkg/adaptive"
	chameleoncrypto "github.com/Hack2p/chameleon/pkg/crypto"
	"github.com/Hack2p/chameleon/pkg/morph"
	"github.com/Hack2p/chameleon/pkg/state"
)

// Config holds packet shaping parameters for the Chameleon wrapper.
type Config struct {
	Profile           BehaviorProfile
	Padding           morph.PaddingConfig
	Jitter            morph.JitterConfig
	SharedSecret      string
	EpochPeriod       time.Duration
	Adaptive          *adaptive.Learner
	AdaptiveStorePath string
	SessionMemoryPath string
}

// Normalizer is the main packet shaping engine.
type Normalizer struct {
	padding morph.PaddingConfig
	jitter  morph.JitterConfig
	profile BehaviorProfile
}

// NewNormalizer creates a validated shaping engine.
func NewNormalizer(cfg Config) (*Normalizer, error) {
	cfg = cfg.resolveProfileDefaults()
	if err := cfg.Padding.Validate(); err != nil {
		return nil, fmt.Errorf("padding config: %w", err)
	}
	if err := cfg.Jitter.Validate(); err != nil {
		return nil, fmt.Errorf("jitter config: %w", err)
	}

	return &Normalizer{
		padding: cfg.Padding,
		jitter:  cfg.Jitter,
		profile: cfg.Profile,
	}, nil
}

// NormalizePacket pads a user payload into a randomized target size and returns the delay to apply.
func (n *Normalizer) NormalizePacket(payload []byte) ([]byte, time.Duration, error) {
	targetSize, err := n.padding.TargetSize(len(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("randomize target size: %w", err)
	}

	normalized, err := n.padding.Normalize(payload, targetSize)
	if err != nil {
		return nil, 0, fmt.Errorf("normalize packet: %w", err)
	}

	delay, err := n.jitter.Next()
	if err != nil {
		return nil, 0, fmt.Errorf("compute jitter delay: %w", err)
	}

	return normalized, delay, nil
}

// Transport is a small UDP wrapper that applies normalization before sending.
type Transport struct {
	conn          net.Conn
	normalizer    *Normalizer
	cipher        *chameleoncrypto.Cipher
	syncer        *state.Sync
	session       *state.Session
	adaptive      *adaptive.Learner
	sessionMemory *adaptive.SessionMemory
	mu            sync.Mutex
}

// NewTransport creates a thin transport wrapper around a net.Conn.
func NewTransport(conn net.Conn, cfg Config) (*Transport, error) {
	normalizer, err := NewNormalizer(cfg)
	if err != nil {
		return nil, err
	}

	learner := cfg.Adaptive
	if learner == nil {
		learner = adaptive.NewLearner()
	}
	if cfg.AdaptiveStorePath != "" {
		if err := learner.SetStorePath(cfg.AdaptiveStorePath); err != nil {
			return nil, fmt.Errorf("set adaptive store path: %w", err)
		}
	}

	var sessionMemory *adaptive.SessionMemory
	if cfg.SessionMemoryPath != "" {
		sessionMemory, err = adaptive.NewSessionMemory(cfg.SessionMemoryPath)
		if err != nil {
			return nil, fmt.Errorf("new session memory: %w", err)
		}
	}

	var cipher *chameleoncrypto.Cipher
	if cfg.SharedSecret != "" {
		cipher, err = chameleoncrypto.NewCipher(cfg.SharedSecret)
		if err != nil {
			return nil, err
		}
	}

	var syncer *state.Sync
	if cfg.SharedSecret != "" {
		syncer = state.NewSync(cfg.SharedSecret)
		if cfg.EpochPeriod > 0 {
			syncer.EpochPeriod = cfg.EpochPeriod
		}
	}

	return &Transport{
		conn:          conn,
		normalizer:    normalizer,
		cipher:        cipher,
		syncer:        syncer,
		session:       state.NewSession(),
		adaptive:      learner,
		sessionMemory: sessionMemory,
	}, nil
}

// Send applies shaped padding and a jitter delay before writing a packet.
func (t *Transport) Send(payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf("payload must not be empty")
	}

	start := time.Now()
	profile := t.profileName()

	data := payload
	if t.cipher != nil {
		sealed, err := t.cipher.Seal(payload)
		if err != nil {
			return fmt.Errorf("seal payload: %w", err)
		}
		data = sealed
	}

	normalized, delay, err := t.normalizer.NormalizePacket(data)
	if err != nil {
		return err
	}

	if t.session != nil {
		period := time.Minute
		if t.syncer != nil && t.syncer.EpochPeriod > 0 {
			period = t.syncer.EpochPeriod
		}
		if err := t.session.Advance(time.Now(), string(profile), period); err != nil {
			return fmt.Errorf("advance session: %w", err)
		}
	}

	if delay > 0 {
		time.Sleep(delay)
	}

	framed, err := EncodeFrame(profile, normalized, len(data))
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}

	t.mu.Lock()
	_, err = t.conn.Write(framed)
	t.mu.Unlock()
	if err != nil {
		t.observeObservation(profile, false, start, payload, normalized)
		return err
	}

	t.observeObservation(profile, true, start, payload, normalized)
	return nil
}

// SendBurst emits a short burst of shaped packets to emulate traffic bursts.
func (t *Transport) SendBurst(payload []byte, count int) error {
	if count <= 0 {
		return fmt.Errorf("count must be positive")
	}

	for i := 0; i < count; i++ {
		if err := t.Send(payload); err != nil {
			return err
		}
	}

	return nil
}

func (t *Transport) observeObservation(profile BehaviorProfile, success bool, started time.Time, payload, normalized []byte) {
	obs := adaptive.Observation{
		Profile:    string(profile),
		Success:    success,
		Latency:    time.Since(started),
		Loss:       0.0,
		Throughput: float64(len(payload)),
		Load:       float64(len(normalized)) / 1024.0,
		SessionID:  "transport",
		At:         time.Now(),
	}
	if !success {
		obs.Loss = 1.0
	}

	if t.adaptive != nil {
		t.adaptive.Observe(obs)
	}
	if t.sessionMemory != nil {
		_ = t.sessionMemory.Observe(obs)
	}
}

func (cfg Config) resolveProfileDefaults() Config {
	if cfg.Profile == "" {
		cfg.Profile = ProfileWebRTC
	}

	defaultCfg := cfg.Profile.Defaults()
	if cfg.Padding == (morph.PaddingConfig{}) {
		cfg.Padding = defaultCfg.Padding
	}
	if cfg.Jitter == (morph.JitterConfig{}) {
		cfg.Jitter = defaultCfg.Jitter
	}

	return cfg
}

func (t *Transport) profileName() BehaviorProfile {
	if t.sessionMemory != nil && len(t.sessionMemory.Profiles) > 0 {
		memoryProfile := t.sessionMemory.BestProfile()
		if memoryProfile != "" && memoryProfile != "webrtc" {
			return BehaviorProfile(memoryProfile)
		}
	}
	if t.adaptive != nil && t.adaptive.HasHistory() {
		decision := t.adaptive.Decide()
		if decision.Profile != "" {
			return BehaviorProfile(decision.Profile)
		}
	}
	if t.syncer == nil || t.syncer.PSK == "" {
		return t.normalizerProfile()
	}

	return BehaviorProfile(t.syncer.ProfileAt(time.Now()))
}

func (t *Transport) normalizerProfile() BehaviorProfile {
	if t == nil || t.normalizer == nil {
		return ProfileWebRTC
	}
	if t.normalizer.profile != "" {
		return t.normalizer.profile
	}
	return ProfileWebRTC
}
