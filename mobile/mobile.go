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
	const probeTimeout = 2500 * time.Millisecond

	selectedTCP := ""
	if strings.TrimSpace(cfg.TLSServer) != "" {
		for _, endpoint := range preferredEndpoints(cfg.TLSServer, "443") {
			probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			_, probeErr := tunnel.ProbeTLSContext(probeCtx, endpoint, cfg.PSK, probeTimeout, tlsCfg)
			cancel()
			if probeErr == nil {
				cfg.TLSServer = endpoint
				selectedTCP = "tls"
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
		}
	}

	if selectedTCP == "" && strings.TrimSpace(cfg.TCPServer) != "" {
		probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		_, probeErr := tunnel.ProbeTCPContext(probeCtx, cfg.TCPServer, cfg.PSK, probeTimeout)
		cancel()
		if probeErr == nil {
			selectedTCP = "tcp"
		}
	}

	if selectedTCP == "" {
		return "ERROR: удалённый Chameleon недоступен по TLS/QUIC/TCP"
	}

	cfg.TCPTransport = selectedTCP

	// QUIC DATAGRAM has a separate ALPN and is a better readiness test for
	// Android UDP than the stream probe. Prefer UDP/443 on filtered mobile
	// networks, then fall back to the profile endpoint. If neither is usable,
	// keep udp_mode=auto so the runtime carries DNS over the selected TCP
	// Chameleon stream without leaking direct UDP.
	cfg.UDPMode = "auto"
	if strings.TrimSpace(cfg.QUICServer) != "" {
		for _, endpoint := range preferredEndpoints(cfg.QUICServer, "443") {
			probeCtx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			session, probeErr := tunnel.DialQUICDatagramSession(
				probeCtx, endpoint, cfg.PSK, 1500*time.Millisecond, tlsCfg,
			)
			cancel()
			if probeErr == nil {
				_ = session.Close()
				cfg.QUICServer = endpoint
				cfg.UDPMode = "quic"
				break
			}
		}
	}

	data, err := clientconfig.JSON(cfg)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return string(data)
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
	controller.Unlock()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		serveErr := app.Serve(ctx, listener)

		controller.Lock()
		if controller.generation == generation {
			if serveErr != nil && ctx.Err() == nil {
				controller.lastError = serveErr.Error()
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

func StatusJSON() string {
	status := map[string]any{
		"version":        buildversion.Current,
		"running":        Running(),
		"owner":          Owner(),
		"listener_ready": ListenerReady(),
		"socks":          "127.0.0.1:1080",
		"last_error":     LastError(),
	}
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Sprintf("{\"version\":%q,\"running\":false}", buildversion.Current)
	}
	return string(data)
}
