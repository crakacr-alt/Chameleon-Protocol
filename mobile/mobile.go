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

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientapp"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
	buildversion "github.com/crakacr-alt/Chameleon-Protocol/pkg/version"
)

var controller struct {
	sync.Mutex
	cancel context.CancelFunc
	done   chan error
}

func Version() string {
	return buildversion.Current
}

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

// Start binds the SOCKS listener before returning. Android can therefore trust
// an empty error as "127.0.0.1:1080 is actually ready", instead of briefly
// showing a connected state while a background bind has already failed.
func Start(configJSON string) string {
	cfg, err := clientconfig.ParseJSON([]byte(configJSON))
	if err != nil {
		return err.Error()
	}

	controller.Lock()
	defer controller.Unlock()
	if controller.cancel != nil {
		return ""
	}

	app, err := clientapp.New(cfg)
	if err != nil {
		return err.Error()
	}

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Sprintf("listen SOCKS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	controller.cancel = cancel
	controller.done = done

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		err := app.Serve(ctx, listener)
		done <- err
		close(done)

		controller.Lock()
		controller.cancel = nil
		controller.done = nil
		controller.Unlock()
	}()

	return ""
}

func Stop() {
	controller.Lock()
	cancel := controller.cancel
	controller.Unlock()
	if cancel != nil {
		cancel()
	}
}

func Running() bool {
	controller.Lock()
	defer controller.Unlock()
	return controller.cancel != nil
}

func StatusJSON() string {
	status := map[string]any{
		"version": buildversion.Current,
		"running": Running(),
		"socks":   "127.0.0.1:1080",
	}
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Sprintf(`{"version":%q,"running":false}`, buildversion.Current)
	}
	return string(data)
}
