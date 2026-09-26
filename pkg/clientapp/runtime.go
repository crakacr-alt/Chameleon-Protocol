package clientapp

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
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
		carrierStore = filepath.Join(cfg.StateDir, "carriers.json")
		dpiStore = filepath.Join(cfg.StateDir, "dpi.json")
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

	openUDP := func(ctx context.Context) (socks5.UDPAssociation, error) {
		mode := strings.ToLower(strings.TrimSpace(cfg.UDPMode))
		if cfg.Mode == clientconfig.ModeProxy && mode == "auto" {
			mode = "quic"
		}
		switch mode {
		case "direct":
			return socks5.NewDirectUDPAssociation(ctx, 8*time.Second), nil
		case "quic":
			if cfg.QUICServer == "" {
				return nil, fmt.Errorf("QUIC UDP requested but quic_server is empty")
			}
			return tunnel.DialQUICDatagramSession(ctx, cfg.QUICServer, cfg.PSK, 8*time.Second, tlsCfg)
		case "auto":
			if cfg.QUICServer != "" {
				return tunnel.DialQUICDatagramSession(ctx, cfg.QUICServer, cfg.PSK, 8*time.Second, tlsCfg)
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
