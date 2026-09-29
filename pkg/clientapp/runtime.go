package clientapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
	adaptiveproxy "github.com/crakacr-alt/Chameleon-Protocol/pkg/proxy"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/socks5"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
)

type Runtime struct {
	Config clientconfig.Config
}

func New(cfg clientconfig.Config) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Runtime{Config: cfg}, nil
}

func (r *Runtime) Serve(ctx context.Context, listener net.Listener) error {
	if r == nil {
		return fmt.Errorf("client runtime is nil")
	}
	if listener == nil {
		return fmt.Errorf("listener is nil")
	}
	cfg := r.Config

	carrierStore := ""
	dpiStore := ""
	if cfg.StateDir != "" {
		scope := adaptiveStateScope(cfg)
		carrierStore = filepath.Join(cfg.StateDir, "profiles", scope, "carriers.json")
		dpiStore = filepath.Join(cfg.StateDir, "profiles", scope, "dpi.json")
	}

	carrierEngine, err := carrier.NewEngine(carrierStore)
	if err != nil {
		return err
	}
	dpiEngine, err := dpi.NewEngine(dpiStore)
	if err != nil {
		return err
	}
	p, err := planner.New(carrierEngine, dpiEngine)
	if err != nil {
		return err
	}

	candidates := carrier.WithQUIC(
		carrier.WithTLS(
			carrier.Defaults("", cfg.TCPServer, ""),
			cfg.TLSServer,
		),
		cfg.QUICServer,
	)
	if cfg.Mode == clientconfig.ModeProxy {
		candidates = withoutDirect(candidates)
	}
	candidates, err = selectTCPCarriers(candidates, cfg.TCPTransport)
	if err != nil {
		return err
	}

	tlsCfg := tunnel.TLSClientConfig{
		ServerName:   cfg.TLSServerName,
		PinnedSHA256: cfg.TLSFingerprint,
	}

	adaptive := &adaptiveproxy.AdaptiveDialer{
		Planner:                  p,
		Carriers:                 candidates,
		DPIStrategies:            dpi.DefaultStrategies(),
		PSK:                      cfg.PSK,
		TLSConfig:                tlsCfg,
		Timeout:                  8 * time.Second,
		DirectCooldown:           cfg.DirectCooldown.Duration(),
		ApplicationFailureWindow: cfg.FailureWindow.Duration(),
		Network:                  networkctx.Detect,
		DisableInitialRace:       strings.ToLower(strings.TrimSpace(cfg.TCPTransport)) != "auto",
	}

	dial := adaptive.DialContext
	if len(cfg.Bypass) > 0 {
		direct := &net.Dialer{Timeout: 8 * time.Second}
		dial = func(ctx context.Context, destination string) (net.Conn, error) {
			if MatchBypass(destination, cfg.Bypass) {
				return direct.DialContext(ctx, "tcp", destination)
			}
			return adaptive.DialContext(ctx, destination)
		}
	}

	var (
		udpMu               sync.Mutex
		quicUnavailableTill time.Time
	)

	openUDP := func(ctx context.Context) (socks5.UDPAssociation, error) {
		mode := strings.ToLower(strings.TrimSpace(cfg.UDPMode))
		switch mode {
		case "direct":
			return socks5.NewDirectUDPAssociation(ctx, 8*time.Second), nil

		case "quic":
			if cfg.QUICServer == "" {
				return nil, fmt.Errorf("QUIC UDP requested but quic_server is empty")
			}
			return tunnel.DialQUICDatagramSession(ctx, cfg.QUICServer, cfg.PSK, 5*time.Second, tlsCfg)

		case "auto":
			if cfg.QUICServer != "" {
				udpMu.Lock()
				blocked := time.Now().Before(quicUnavailableTill)
				udpMu.Unlock()

				if !blocked {
					association, err := tunnel.DialQUICDatagramSession(
						ctx, cfg.QUICServer, cfg.PSK, 3*time.Second, tlsCfg,
					)
					if err == nil {
						udpMu.Lock()
						quicUnavailableTill = time.Time{}
						udpMu.Unlock()
						return association, nil
					}

					// A mobile network can block UDP/QUIC while TCP/TLS still works.
					// Remember that briefly so every DNS request doesn't pay the
					// full QUIC timeout before using the stream fallback.
					udpMu.Lock()
					quicUnavailableTill = time.Now().Add(60 * time.Second)
					udpMu.Unlock()
				}
			}

			if cfg.Mode == clientconfig.ModeProxy {
				// Keep DNS inside Chameleon by carrying the RFC 1928 UDP DNS
				// payload over a normal TCP tunnel. Other UDP is rejected so
				// applications can fall back to TCP instead of leaking direct.
				return socks5.NewDNSOverTCPAssociation(dial), nil
			}
			return socks5.NewDirectUDPAssociation(ctx, 8*time.Second), nil

		default:
			return nil, fmt.Errorf("unsupported UDP mode %q", cfg.UDPMode)
		}
	}

	server := &socks5.Server{
		Dial:    dial,
		OpenUDP: openUDP,
	}
	return server.Serve(ctx, listener)
}

func (r *Runtime) ListenAndServe(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("client runtime is nil")
	}
	listener, err := net.Listen("tcp", r.Config.Listen)
	if err != nil {
		return fmt.Errorf("listen SOCKS: %w", err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	err = r.Serve(ctx, listener)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func withoutDirect(candidates []carrier.Candidate) []carrier.Candidate {
	out := make([]carrier.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Kind == carrier.KindDirect {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func selectTCPCarriers(candidates []carrier.Candidate, transport string) ([]carrier.Candidate, error) {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "", "auto":
		return candidates, nil
	case "tls":
		for _, candidate := range candidates {
			if candidate.Kind == carrier.KindChameleonTLS {
				return []carrier.Candidate{candidate}, nil
			}
		}
		return nil, fmt.Errorf("tcp_transport=tls but tls_server is not configured")
	case "quic":
		for _, candidate := range candidates {
			if candidate.Kind == carrier.KindChameleonQUIC {
				return []carrier.Candidate{candidate}, nil
			}
		}
		return nil, fmt.Errorf("tcp_transport=quic but quic_server is not configured")
	case "tcp":
		for _, candidate := range candidates {
			if candidate.Kind == carrier.KindChameleonTCP {
				return []carrier.Candidate{candidate}, nil
			}
		}
		return nil, fmt.Errorf("tcp_transport=tcp but tcp_server is not configured")
	default:
		return nil, fmt.Errorf("unsupported tcp_transport %q", transport)
	}
}

func adaptiveStateScope(cfg clientconfig.Config) string {
	identity := strings.Join([]string{
		strings.TrimSpace(cfg.Server),
		strings.TrimSpace(cfg.QUICServer),
		strings.TrimSpace(cfg.TLSServer),
		strings.TrimSpace(cfg.TCPServer),
		strings.TrimSpace(cfg.TLSFingerprint),
		strings.TrimSpace(cfg.TLSServerName),
		strings.TrimSpace(cfg.TCPTransport),
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:8])
}
