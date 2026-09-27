package clientconfig

import (
	"encoding/json"
	"fmt"
)

// ParseJSON loads a client configuration from bytes without requiring a file.
// Android and other embedded clients use this path so the core config validation
// stays identical across desktop and mobile front-ends.
func ParseJSON(data []byte) (Config, error) {
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Migrate(); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// JSON returns the canonical serialized representation of a validated config.
func JSON(cfg Config) ([]byte, error) {
	if err := cfg.Migrate(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return append(data, '\n'), nil
}
