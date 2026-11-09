package clientconfig

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// PrepareBridge makes a TCP-only, fail-closed Chameleon profile that can run
// alongside an existing system VPN or any other available external route.
// The local SOCKS listener remains the only traffic entry point: Bridge
// never acquires an Android VpnService and never changes global routing.
//
// endpoint is an optional host:port override for the first hop (for example a
// Tailscale IP). An empty endpoint preserves the imported server address.
// Authentication credentials and the TLS certificate pin remain untouched.
func PrepareBridge(cfg Config, endpoint string) (Config, error) {
	if err := cfg.Migrate(); err != nil {
		return Config{}, err
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint != "" {
		host, port, err := net.SplitHostPort(endpoint)
		if err != nil || strings.TrimSpace(host) == "" {
			return Config{}, fmt.Errorf("bridge endpoint must be host:port (IPv6: [address]:port)")
		}
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return Config{}, fmt.Errorf("bridge endpoint port must be 1-65535")
		}
	}

	cfg.Mode = ModeProxy
	// QUIC is not used in this conservative compatibility mode. SOCKS5 DNS
	// travels over authenticated Chameleon TCP instead of leaking direct UDP.
	cfg.QUICServer = ""
	cfg.UDPMode = "auto"

	switch {
	case strings.TrimSpace(cfg.TLSServer) != "":
		if endpoint != "" {
			cfg.TLSServer = endpoint
		}
		cfg.TCPServer = ""
		cfg.TCPTransport = "tls"
	case strings.TrimSpace(cfg.TCPServer) != "":
		if endpoint != "" {
			cfg.TCPServer = endpoint
		}
		cfg.TCPTransport = "tcp"
	default:
		return Config{}, fmt.Errorf("bridge requires a TLS or TCP Chameleon endpoint; QUIC-only profile is unsupported")
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
