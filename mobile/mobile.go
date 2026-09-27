// Package mobile is the small gomobile-facing API for the Android client.
//
// The UI intentionally depends on this tiny surface instead of importing
// Chameleon internals. That keeps Android presentation code separate from the
// transport engine and lets desktop/mobile clients share the same core.
package mobile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// Version returns the embedded Chameleon protocol version.
func Version() string {
	return buildversion.Current
}

// BuildConfig converts the server-generated client profile into the canonical
// JSON config consumed by the shared runtime. On error the returned string
// begins with "ERROR:" so the Java bridge can report the problem without a
// second binding type.
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
	// Keep localhost out of adaptive routing. Extra LAN rules can be added later
	// from the Android settings screen without changing the protocol.
	cfg.Bypass = []string{"localhost", "127.0.0.0/8", "::1/128"}

	data, err := clientconfig.JSON(cfg)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return string(data)
}

// ValidateConfig returns an empty string when the JSON configuration is valid.
func ValidateConfig(configJSON string) string {
	_, err := clientconfig.ParseJSON([]byte(configJSON))
	if err != nil {
		return err.Error()
	}
	return ""
}

// Start launches the shared Chameleon SOCKS runtime in the background.
// Android owns lifecycle/foreground-service policy; Go owns networking.
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

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	controller.cancel = cancel
	controller.done = done

	go func() {
		err := app.ListenAndServe(ctx)
		done <- err
		close(done)

		controller.Lock()
		controller.cancel = nil
		controller.done = nil
		controller.Unlock()
	}()

	return ""
}

// Stop requests a graceful shutdown of the local proxy.
func Stop() {
	controller.Lock()
	cancel := controller.cancel
	controller.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Running reports whether the shared runtime currently owns an active context.
func Running() bool {
	controller.Lock()
	defer controller.Unlock()
	return controller.cancel != nil
}

// StatusJSON is deliberately secret-free and safe to show in the Android UI.
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
