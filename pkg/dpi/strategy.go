package dpi

import (
	"fmt"
	"strings"
	"time"
)

// Technique describes one traffic handling idea.
// The generic Go executor can safely do direct and split writes.
// Packet-level techniques are kept as capabilities for a future
// Linux/router backend where raw packet control is available.
type Technique string

const (
	TechniqueDirect     Technique = "direct"
	TechniqueSplit      Technique = "split"
	TechniquePacedSplit Technique = "paced_split"
	TechniqueFake       Technique = "fake"
	TechniqueDisorder   Technique = "disorder"
)

// Strategy describes one candidate that the adaptive engine may select.
type Strategy struct {
	Name                  string        `json:"name"`
	Techniques            []Technique   `json:"techniques"`
	SplitPoints           []int         `json:"split_points,omitempty"`
	DelayBetweenFragments time.Duration `json:"delay_between_fragments,omitempty"`
	Cost                  float64       `json:"cost"`
	RequiresPacketControl bool          `json:"requires_packet_control,omitempty"`
}

// Validate catches configuration mistakes before a strategy reaches the wire.
func (s Strategy) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("strategy name must not be empty")
	}
	if len(s.Techniques) == 0 {
		return fmt.Errorf("strategy %q has no techniques", s.Name)
	}
	if s.Cost < 0 {
		return fmt.Errorf("strategy %q has negative cost", s.Name)
	}
	if s.DelayBetweenFragments < 0 {
		return fmt.Errorf("strategy %q has negative delay", s.Name)
	}
	for _, p := range s.SplitPoints {
		if p <= 0 {
			return fmt.Errorf("strategy %q has invalid split point %d", s.Name, p)
		}
	}
	return nil
}

// DefaultStrategies are cheap, cross-platform strategies.
// Direct is intentionally first: Chameleon should not modify traffic
// when the normal path already works.
func DefaultStrategies() []Strategy {
	return []Strategy{
		{
			Name:       "direct",
			Techniques: []Technique{TechniqueDirect},
			Cost:       0,
		},
		{
			Name:        "split-early",
			Techniques:  []Technique{TechniqueSplit},
			SplitPoints: []int{1},
			Cost:        0.35,
		},
		{
			Name:                  "paced-split",
			Techniques:            []Technique{TechniquePacedSplit},
			SplitPoints:           []int{1},
			DelayBetweenFragments: 2 * time.Millisecond,
			Cost:                  0.65,
		},
	}
}

// PacketControlStrategies are ideas inspired by packet-level DPI desync tools.
// They are intentionally not executed by the generic userspace writer.
// A platform backend must explicitly opt in and implement the packet semantics.
func PacketControlStrategies() []Strategy {
	return []Strategy{
		{
			Name:                  "fake-then-split",
			Techniques:            []Technique{TechniqueFake, TechniqueSplit},
			SplitPoints:           []int{1},
			Cost:                  1.25,
			RequiresPacketControl: true,
		},
		{
			Name:                  "disorder-split",
			Techniques:            []Technique{TechniqueDisorder, TechniqueSplit},
			SplitPoints:           []int{1},
			Cost:                  1.5,
			RequiresPacketControl: true,
		},
	}
}
