package clientconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = 1

type Mode string

const (
	ModeSmart Mode = "smart"
	ModeProxy Mode = "proxy"
)

type Config struct {
	SchemaVersion  int      `json:"schema_version"`
	Mode           Mode     `json:"mode"`
	Listen         string   `json:"listen"`
	Server         string   `json:"server,omitempty"`
	QUICServer     string   `json:"quic_server,omitempty"`
	TLSServer      string   `json:"tls_server,omitempty"`
	TCPServer      string   `json:"tcp_server,omitempty"`
	PSK            string   `json:"psk"`
	TLSFingerprint string   `json:"tls_fingerprint,omitempty"`
	TLSServerName  string   `json:"tls_server_name,omitempty"`
	UDPMode        string   `json:"udp_mode"`
	StateDir       string   `json:"state_dir,omitempty"`
	DirectCooldown Duration `json:"direct_cooldown"`
	FailureWindow  Duration `json:"failure_window"`
	Bypass         []string `json:"bypass,omitempty"`
}

type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}
	value, err := time.ParseDuration(text)
	if err != nil {
		return err
	}
	*d = Duration(value)
	return nil
}

func Default() Config {
	return Config{
		SchemaVersion:  SchemaVersion,
		Mode:           ModeSmart,
		Listen:         "127.0.0.1:1080",
		UDPMode:        "auto",
		DirectCooldown: Duration(10 * time.Minute),
		FailureWindow:  Duration(12 * time.Second),
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
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

func Save(path string, cfg Config) error {
	if err := cfg.Migrate(); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) Migrate() error {
	if c.SchemaVersion == 0 {
		c.SchemaVersion = 1
	}
	if c.SchemaVersion > SchemaVersion {
		return fmt.Errorf("config schema %d is newer than supported schema %d", c.SchemaVersion, SchemaVersion)
	}
	if c.Mode == "" {
		c.Mode = ModeSmart
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:1080"
	}
	if c.UDPMode == "" {
		c.UDPMode = "auto"
	}
	if c.DirectCooldown == 0 {
		c.DirectCooldown = Duration(10 * time.Minute)
	}
	if c.FailureWindow == 0 {
		c.FailureWindow = Duration(12 * time.Second)
	}
	return nil
}

func (c Config) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported config schema %d", c.SchemaVersion)
	}
	switch c.Mode {
	case ModeSmart, ModeProxy:
	default:
		return fmt.Errorf("unsupported mode %q", c.Mode)
	}
	if strings.TrimSpace(c.Listen) == "" {
		return fmt.Errorf("listen address is required")
	}
	switch strings.ToLower(strings.TrimSpace(c.UDPMode)) {
	case "auto", "direct", "quic":
	default:
		return fmt.Errorf("unsupported udp_mode %q", c.UDPMode)
	}
	if c.Mode == ModeProxy && c.QUICServer == "" && c.TLSServer == "" && c.TCPServer == "" {
		return fmt.Errorf("proxy mode requires at least one Chameleon server endpoint")
	}
	if (c.QUICServer != "" || c.TLSServer != "" || c.TCPServer != "") && strings.TrimSpace(c.PSK) == "" {
		return fmt.Errorf("psk is required when tunnel endpoints are configured")
	}
	if c.DirectCooldown.Duration() < 0 || c.FailureWindow.Duration() < 0 {
		return fmt.Errorf("durations must not be negative")
	}
	return nil
}
