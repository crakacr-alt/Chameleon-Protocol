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

// Diagnose runs real remote first-hop checks and returns JSON suitable for the
// Android Doctor screen. The PSK is used for authentication but is never
// included in the returned report.
func Diagnose(configJSON string) string {
	cfg, err := clientconfig.ParseJSON([]byte(configJSON))
	if err != nil {
		data, _ := json.Marshal(map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return string(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	checks := clientapp.ProbeTransports(ctx, cfg, 1, 5*time.Second)

	ok := len(checks) > 0
	for _, check := range checks {
		if !check.OK {
			ok = false
		}
	}
	data, err := json.Marshal(map[string]any{
		"ok":     ok,
		"checks": checks,
	})
	if err != nil {
		return fmt.Sprintf("{\"ok\":false,\"error\":%q}", err.Error())
	}
	return string(data)
}
