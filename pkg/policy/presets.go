package policy

import (
	"fmt"
	"strings"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
)

type Preset string

const (
	PresetAuto      Preset = "auto"
	PresetFast      Preset = "fast"
	PresetStable    Preset = "stable"
	PresetGaming    Preset = "gaming"
	PresetStreaming Preset = "streaming"
)

func Names() []string {
	return []string{
		string(PresetAuto),
		string(PresetFast),
		string(PresetStable),
		string(PresetGaming),
		string(PresetStreaming),
	}
}

func Normalize(value string) (Preset, error) {
	preset := Preset(strings.ToLower(strings.TrimSpace(value)))
	if preset == "" {
		preset = PresetAuto
	}
	switch preset {
	case PresetAuto, PresetFast, PresetStable, PresetGaming, PresetStreaming:
		return preset, nil
	default:
		return "", fmt.Errorf("unknown preset %q", value)
	}
}

// CarrierPolicy converts a user-facing preset into actual adaptive scoring
// weights. Presets never bypass measured evidence; they only change what
// "better" means for the current user goal.
func CarrierPolicy(value string) (carrier.ScorePolicy, error) {
	preset, err := Normalize(value)
	if err != nil {
		return carrier.ScorePolicy{}, err
	}

	switch preset {
	case PresetFast:
		return carrier.ScorePolicy{
			LatencyScale:    1.55,
			JitterScale:     1.15,
			ThroughputScale: 0.9,
			CostScale:       1.1,
		}, nil
	case PresetStable:
		return carrier.ScorePolicy{
			LatencyScale:    0.9,
			JitterScale:     1.8,
			ThroughputScale: 1.1,
			CostScale:       0.85,
		}, nil
	case PresetGaming:
		return carrier.ScorePolicy{
			LatencyScale:    2.2,
			JitterScale:     2.4,
			ThroughputScale: 0.45,
			CostScale:       0.75,
		}, nil
	case PresetStreaming:
		return carrier.ScorePolicy{
			LatencyScale:    0.55,
			JitterScale:     0.8,
			ThroughputScale: 2.25,
			CostScale:       0.75,
		}, nil
	default:
		return carrier.DefaultScorePolicy(), nil
	}
}
