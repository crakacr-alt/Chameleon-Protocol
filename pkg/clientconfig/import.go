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
	var explicitQUIC, explicitTLS, explicitTCP bool
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
			explicitQUIC = true
		case "CHAMELEON_TLS_SERVER":
			cfg.TLSServer = value
			explicitTLS = true
		case "CHAMELEON_TCP_SERVER":
			cfg.TCPServer = value
			explicitTCP = true
		case "CHAMELEON_TUNNEL_PSK":
			cfg.PSK = value
		case "CHAMELEON_CLIENT_ID":
			cfg.ClientID = value
		case "CHAMELEON_CLIENT_SECRET":
			cfg.ClientSecret = value
		case "CHAMELEON_TLS_FINGERPRINT":
			cfg.TLSFingerprint = value
		case "CHAMELEON_TLS_SERVER_NAME":
			cfg.TLSServerName = value
		case "CHAMELEON_TCP_TRANSPORT":
			cfg.TCPTransport = value
		case "CHAMELEON_UDP_MODE":
			cfg.UDPMode = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read profile: %w", err)
	}
	// Legacy profiles that only contain CHAMELEON_SERVER predate explicit
	// transport declarations. Preserve their historical all-carrier behavior.
	//
	// Once a profile explicitly declares at least one transport, however, do
	// not invent the missing ones. A TLS listener is not automatically a raw
	// Chameleon TCP listener, and a TCP relay does not imply UDP/QUIC support.
	if cfg.Server != "" && !explicitQUIC && !explicitTLS && !explicitTCP {
		cfg.QUICServer = cfg.Server
		cfg.TLSServer = cfg.Server
		cfg.TCPServer = cfg.Server
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
