package control

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientapp"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/policy"
	buildversion "github.com/crakacr-alt/Chameleon-Protocol/pkg/version"
)

type Server struct {
	Runtime    *clientapp.Runtime
	ConfigPath string
}

type Status struct {
	Version      string             `json:"version"`
	Mode         clientconfig.Mode  `json:"mode"`
	Preset       string             `json:"preset"`
	SOCKS        string             `json:"socks"`
	Network      networkctx.Context `json:"network"`
	QUIC         bool               `json:"quic"`
	TLS          bool               `json:"tls"`
	TCP          bool               `json:"tcp"`
	UDPMode      string             `json:"udp_mode"`
	BypassRules  int                `json:"bypass_rules"`
}

func (s *Server) Handler() (http.Handler, error) {
	if s == nil || s.Runtime == nil {
		return nil, fmt.Errorf("control runtime is required")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/presets", s.handlePresets)
	mux.HandleFunc("/api/preset", s.handlePreset)
	return localOnly(mux), nil
}

func (s *Server) Serve(ctx context.Context, address string) error {
	if strings.TrimSpace(address) == "" {
		address = "127.0.0.1:8765"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid control address: %w", err)
	}
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("control API may only listen on loopback")
	}

	handler, err := s.Handler()
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := s.Runtime.ConfigSnapshot()
	network, _ := networkctx.Detect()
	writeJSON(w, Status{
		Version:     buildversion.Current,
		Mode:        cfg.Mode,
		Preset:      cfg.Preset,
		SOCKS:       cfg.Listen,
		Network:     network,
		QUIC:        cfg.QUICServer != "",
		TLS:         cfg.TLSServer != "",
		TCP:         cfg.TCPServer != "",
		UDPMode:     cfg.UDPMode,
		BypassRules: len(cfg.Bypass),
	})
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"presets": policy.Names()})
}

func (s *Server) handlePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Preset string `json:"preset"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.Runtime.SetPreset(body.Preset); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.ConfigPath != "" {
		if err := clientconfig.Save(s.ConfigPath, s.Runtime.ConfigSnapshot()); err != nil {
			http.Error(w, "preset applied but config save failed", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]any{
		"ok":     true,
		"preset": s.Runtime.ConfigSnapshot().Preset,
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
