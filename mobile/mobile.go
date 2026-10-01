// Package mobile is the small gomobile-facing API for the Android client.
package mobile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientapp"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
	buildversion "github.com/crakacr-alt/Chameleon-Protocol/pkg/version"
)

var controller struct {
	sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	owner      string
	listen     string
	lastError  string
	stage      string
	generation uint64
}

func Version() string { return buildversion.Current }

func BuildConfig(profileText, stateDir, mode string) string {
	cfg, err := clientconfig.ImportProfile(bytes.NewBufferString(profileText))
	if err != nil {
		return "ERROR: " + err.Error()
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "smart":
		cfg.Mode = clientconfig.ModeSmart
	case "proxy":
		cfg.Mode = clientconfig.ModeProxy
	default:
		return "ERROR: unsupported mode " + mode
	}

	cfg.StateDir = stateDir
	cfg.Bypass = []string{"localhost", "127.0.0.0/8", "::1/128"}

	data, err := clientconfig.JSON(cfg)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return string(data)
}

func PrepareVPNConfig(configJSON string) string {
	cfg, err := clientconfig.ParseJSON([]byte(configJSON))
	if err != nil {
		return "ERROR: " + err.Error()
	}
	cfg.Mode = clientconfig.ModeProxy

	tlsCfg := tunnel.TLSClientConfig{
		ServerName:   cfg.TLSServerName,
		PinnedSHA256: cfg.TLSFingerprint,
	}
	// Mobile radio wake-up plus the Server-2 -> WireGuard -> Server-1 hop can
	// exceed a couple of seconds even when the service is healthy. A short
	// preflight deadline made Android report a TLS context deadline before the
	// first authenticated response could return.
	const probeTimeout = 8 * time.Second

	selectedTCP := ""
	probeFailures := make([]string, 0, 4)
	if strings.TrimSpace(cfg.TLSServer) != "" {
		// Some mobile paths accept the TCP connection and the first TLS record
		// byte, but suppress the rest of a tightly packed ClientHello. Probe the
		// paced variant first, then fall back to the low-latency split-early
		// variant used by the adaptive runtime.
		for _, strategy := range mobileProbeStrategies() {
			probeDial := func(ctx context.Context, address string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: probeTimeout}
				conn, err := dialer.DialContext(ctx, "tcp", address)
				if err != nil {
					return nil, err
				}
				return dpi.NewFirstWriteConn(conn, strategy), nil
			}
			for _, endpoint := range preferredEndpoints(cfg.TLSServer, "443") {
				probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
				_, probeErr := tunnel.ProbeTLSWithDialer(probeCtx, endpoint, cfg.PSK, probeTimeout, tlsCfg, probeDial)
				cancel()
				if probeErr == nil {
					cfg.TLSServer = endpoint
					selectedTCP = "tls"
					break
				}
				probeFailures = append(probeFailures, fmt.Sprintf("TLS %s (%s): %v", endpoint, strategy.Name, probeErr))
			}
			if selectedTCP != "" {
				break
			}
		}
	}

	if selectedTCP == "" && strings.TrimSpace(cfg.QUICServer) != "" {
		for _, endpoint := range preferredEndpoints(cfg.QUICServer, "443") {
			probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			_, probeErr := tunnel.ProbeQUICContext(probeCtx, endpoint, cfg.PSK, probeTimeout, tlsCfg)
			cancel()
			if probeErr == nil {
				cfg.QUICServer = endpoint
				selectedTCP = "quic"
				break
			}
			probeFailures = append(probeFailures, fmt.Sprintf("QUIC %s: %v", endpoint, probeErr))
		}
	}

	if selectedTCP == "" && strings.TrimSpace(cfg.TCPServer) != "" {
		probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		_, probeErr := tunnel.ProbeTCPContext(probeCtx, cfg.TCPServer, cfg.PSK, probeTimeout)
		cancel()
		if probeErr == nil {
			selectedTCP = "tcp"
		} else {
			probeFailures = append(probeFailures, fmt.Sprintf("TCP %s: %v", cfg.TCPServer, probeErr))
		}
	}

	if selectedTCP == "" {
		if len(probeFailures) == 0 {
			return "ERROR: удалённый Chameleon недоступен: в профиле нет transport endpoint"
		}
		return "ERROR: удалённый Chameleon недоступен; " + strings.Join(probeFailures, "; ")
	}

	cfg.TCPTransport = selectedTCP

	// QUIC DATAGRAM has a separate ALPN and is a better readiness test for
	// Android UDP than the stream probe. Prefer UDP/443 on filtered mobile
	// networks, then fall back to the profile endpoint. If neither is usable,
	// keep udp_mode=auto so the runtime carries DNS over the selected TCP
	// Chameleon stream without leaking direct UDP.
	cfg.UDPMode = "auto"
	udpReady := false
	if strings.TrimSpace(cfg.QUICServer) != "" {
		for _, endpoint := range preferredEndpoints(cfg.QUICServer, "443") {
			const udpProbeTimeout = 6 * time.Second
			probeCtx, cancel := context.WithTimeout(context.Background(), udpProbeTimeout)
			session, probeErr := tunnel.DialQUICDatagramSession(
				probeCtx, endpoint, cfg.PSK, udpProbeTimeout, tlsCfg,
			)
			cancel()
			if probeErr == nil {
				_ = session.Close()
				cfg.QUICServer = endpoint
				cfg.UDPMode = "quic"
				udpReady = true
				break
			}
		}
	}
	// Do not keep a dead QUIC endpoint in an authenticated mobile config.
	// Otherwise every DNS request can pay another UDP timeout even though the
	// preflight has already proved that this network/relay has no QUIC path.
	if !udpReady && selectedTCP != "quic" {
		cfg.QUICServer = ""
	}

	data, err := clientconfig.JSON(cfg)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return string(data)
}

func mobileProbeStrategies() []dpi.Strategy {
	var paced, split dpi.Strategy
	for _, strategy := range dpi.DefaultStrategies() {
		switch strategy.Name {
		case "paced-split":
			// A few mobile middleboxes need a visible inter-fragment gap. Keep
			// this limited to the one ClientHello probe; application traffic
			// still uses the same paced first-flight behavior in adaptive
			// runtime after the preflight has proved it works.
			strategy.Name = "paced-split-mobile"
			strategy.DelayBetweenFragments = 25 * time.Millisecond
			paced = strategy
		case "split-early":
			split = strategy
		}
	}
	strategies := make([]dpi.Strategy, 0, 2)
	if paced.Name != "" {
		strategies = append(strategies, paced)
	}
	if split.Name != "" {
		strategies = append(strategies, split)
	}
	return strategies
}

func preferredEndpoints(endpoint, preferredPort string) []string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || strings.TrimSpace(host) == "" {
		return []string{endpoint}
	}
	preferred := net.JoinHostPort(host, preferredPort)
	if port == preferredPort || preferred == endpoint {
		return []string{endpoint}
	}
	return []string{preferred, endpoint}
}

func ValidateConfig(configJSON string) string {
	_, err := clientconfig.ParseJSON([]byte(configJSON))
	if err != nil {
		return err.Error()
	}
	return ""
}

func Start(configJSON string) string {
	return StartOwned(configJSON, "legacy")
}

// StartOwned starts the shared SOCKS runtime for one lifecycle owner.
// A stale Android sidecar callback can therefore no longer stop a newer VPN runtime.
func StartOwned(configJSON, owner string) string {
	cfg, err := clientconfig.ParseJSON([]byte(configJSON))
	if err != nil {
		return err.Error()
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "legacy"
	}

	app, err := clientapp.New(cfg)
	if err != nil {
		setControllerError("client runtime init", err)
		return err.Error()
	}

	controller.Lock()
	if controller.cancel != nil {
		current := controller.owner
		controller.Unlock()
		if current == owner {
			return ""
		}
		return fmt.Sprintf("runtime already owned by %s", current)
	}

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		controller.Unlock()
		setControllerError("listener bind", err)
		return fmt.Sprintf("listen SOCKS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	controller.generation++
	generation := controller.generation
	controller.cancel = cancel
	controller.done = done
	controller.owner = owner
	controller.listen = listener.Addr().String()
	controller.lastError = ""
	controller.stage = "listener-ready"
	controller.Unlock()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		controller.Lock()
		if controller.generation == generation {
			controller.stage = "runtime-serving"
		}
		controller.Unlock()
		serveErr := app.Serve(ctx, listener)

		controller.Lock()
		if controller.generation == generation {
			if serveErr != nil && ctx.Err() == nil {
				controller.lastError = "listener stopped: " + serveErr.Error()
				controller.stage = "listener-stopped"
			} else if ctx.Err() != nil {
				controller.stage = "stopped"
			}
			controller.cancel = nil
			controller.done = nil
			controller.owner = ""
			controller.listen = ""
		}
		controller.Unlock()
		close(done)
	}()

	return ""
}

func setControllerError(stage string, err error) {
	controller.Lock()
	controller.stage = stage
	if err != nil {
		controller.lastError = err.Error()
	}
	controller.Unlock()
}

func Stop() { stopOwned("") }

func StopOwned(owner string) { stopOwned(strings.TrimSpace(owner)) }

func stopOwned(owner string) {
	controller.Lock()
	if controller.cancel == nil {
		controller.Unlock()
		return
	}
	if owner != "" && controller.owner != owner {
		controller.Unlock()
		return
	}
	cancel := controller.cancel
	done := controller.done
	controller.Unlock()

	cancel()
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			controller.Lock()
			if controller.cancel != nil {
				controller.lastError = "runtime stop timed out"
			}
			controller.Unlock()
		}
	}
}

func Running() bool {
	controller.Lock()
	defer controller.Unlock()
	return controller.cancel != nil
}

func Owner() string {
	controller.Lock()
	defer controller.Unlock()
	return controller.owner
}

func ListenerReady() bool {
	controller.Lock()
	running := controller.cancel != nil
	address := controller.listen
	controller.Unlock()

	if !running || strings.TrimSpace(address) == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func LastError() string {
	controller.Lock()
	defer controller.Unlock()
	return controller.lastError
}

// Stage reports the last lifecycle stage for Android diagnostics.
func Stage() string {
	controller.Lock()
	defer controller.Unlock()
	return controller.stage
}

func StatusJSON() string {
	status := map[string]any{
		"version":        buildversion.Current,
		"running":        Running(),
		"owner":          Owner(),
		"listener_ready": ListenerReady(),
		"socks":          "127.0.0.1:1080",
		"last_error":     LastError(),
		"stage":          Stage(),
	}
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Sprintf("{\"version\":%q,\"running\":false}", buildversion.Current)
	}
	return string(data)
}
