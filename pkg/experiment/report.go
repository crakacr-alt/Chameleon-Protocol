package experiment

import (
	"fmt"

	"github.com/Hack2p/chameleon/pkg/core"
)

// CompareProfiles runs a small multi-scenario comparison and returns a map
// keyed by profile string for a report-friendly matrix.
func CompareProfiles(scenarios ...Scenario) (map[string]*Metrics, error) {
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("at least one scenario is required")
	}

	results := make(map[string]*Metrics, len(scenarios))
	for _, scenario := range scenarios {
		metrics, err := scenario.Run()
		if err != nil {
			return nil, fmt.Errorf("run scenario: %w", err)
		}
		results[string(scenario.Profile)] = metrics
	}

	return results, nil
}

// CompareProfilesReport returns a small textual matrix summary.
func CompareProfilesReport(scenarios ...Scenario) (string, error) {
	matrix, err := CompareProfiles(scenarios...)
	if err != nil {
		return "", err
	}

	report := "profile comparison\n"
	for profile, metrics := range matrix {
		report += fmt.Sprintf("%s: throughput=%.2f loss=%.4f mean_latency=%s\n", profile, metrics.Throughput, metrics.LossRate, metrics.MeanLatency)
	}
	return report, nil
}

// ProfileMatrix is kept small and explicit for future report rendering.
type ProfileMatrix map[string]*Metrics

// NewProfileMatrix creates an empty comparison matrix.
func NewProfileMatrix() ProfileMatrix {
	return make(ProfileMatrix)
}

// Add stores a scored profile result into the matrix.
func (m ProfileMatrix) Add(profile core.BehaviorProfile, metrics *Metrics) {
	if m == nil {
		return
	}
	m[string(profile)] = metrics
}
