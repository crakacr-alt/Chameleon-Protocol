package clientconfig

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ImportProfile reads the root-only profile produced by install-server.sh.
// Unknown keys and explanatory lines are ignored so the profile can grow
// without breaking older clients.
func ImportProfile(r io.Reader) (Config, error) {
	cfg := Default()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "Linux ") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "CHAMELEON_SERVER":
			cfg.Server = value
		case "CHAMELEON_QUIC_SERVER":
			cfg.QUICServer = value
		case "CHAMELEON_TLS_SERVER":
			cfg.TLSServer = value
		case "CHAMELEON_TCP_SERVER":
			cfg.TCPServer = value
		case "CHAMELEON_TUNNEL_PSK":
			cfg.PSK = value
		case "CHAMELEON_TLS_FINGERPRINT":
			cfg.TLSFingerprint = value
		case "CHAMELEON_TLS_SERVER_NAME":
			cfg.TLSServerName = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read profile: %w", err)
	}
	if cfg.QUICServer == "" && cfg.Server != "" {
		cfg.QUICServer = cfg.Server
	}
	if cfg.TLSServer == "" && cfg.Server != "" {
		cfg.TLSServer = cfg.Server
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
