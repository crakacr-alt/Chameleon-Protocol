package experiment

import (
	"math/rand"
	"time"
)

// SimpleClassifierSim is a toy statistical classifier that scores entropy
// and timing features to estimate detectability. This is a lightweight
// simulation for experiment harness; real ML models should be used for
// robust evaluation.
type SimpleClassifierSim struct {
	rng *rand.Rand
}

func NewSimpleClassifierSim(seed int64) *SimpleClassifierSim {
	return &SimpleClassifierSim{rng: rand.New(rand.NewSource(seed))}
}

// Score returns a detectability score in [0,1], where 1 means highly detectable.
// It uses a naive model: lower entropy and regular timing => higher score.
func (s *SimpleClassifierSim) Score(payloadEntropy float64, jitterMs float64) float64 {
	// payloadEntropy expected in [0..8] bits/byte approx; map to [0..1]
	e := payloadEntropy / 8.0
	if e < 0 {
		e = 0
	}
	if e > 1 {
		e = 1
	}

	t := jitterMs / 50.0 // normalize: 50ms is moderate jitter
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	// base detectability reduces with entropy and increases with timing regularity
	base := (1.0 - e)*0.7 + (1.0-t)*0.3

	// add some randomness
	noise := s.rng.Float64() * 0.1
	score := base + noise
	if score > 1 {
		score = 1
	}
	return score
}
