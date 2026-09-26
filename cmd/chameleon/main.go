package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientapp"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/probe"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
	buildversion "github.com/crakacr-alt/Chameleon-Protocol/pkg/version"
)

type doctorReport struct {
	Version string                 `json:"version"`
	Config  string                 `json:"config"`
	Network networkctx.Context     `json:"network"`
	Checks  []doctorCheck          `json:"checks"`
}

type doctorCheck struct {
	Name    string        `json:"name"`
	Target  string        `json:"target,omitempty"`
	OK      bool          `json:"ok"`
	Latency time.Duration `json:"latency,omitempty"`
	Error   string        `json:"error,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version", "--version", "-version":
		fmt.Println(buildversion.Current)
	case "import":
		runImport(os.Args[2:])
	case "connect":
		runConnect(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "show":
		runShow(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		exitErr(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func usage() {
	fmt.Printf(`Chameleon %s

Usage:
  chameleon import PROFILE [--config PATH] [--mode smart|proxy]
  chameleon connect [--config PATH]
  chameleon status [--config PATH]
  chameleon doctor [--config PATH] [--json]
  chameleon show [--config PATH]
  chameleon version

Normal first setup:
  chameleon import client-profile.txt
  chameleon doctor
  chameleon connect

`, buildversion.Current)
}

func runImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "destination config path")
	mode := fs.String("mode", string(clientconfig.ModeSmart), "smart or proxy")
	stateDir := fs.String("state-dir", defaultStateDir(), "adaptive state directory")
	if err := fs.Parse(args); err != nil {
		exitErr(err)
	}
	if fs.NArg() != 1 {
		exitErr(fmt.Errorf("import requires exactly one server profile path"))
	}

	file, err := os.Open(fs.Arg(0))
	if err != nil {
		exitErr(fmt.Errorf("open profile: %w", err))
	}
	defer file.Close()

	cfg, err := clientconfig.ImportProfile(file)
	if err != nil {
		exitErr(err)
	}
	cfg.Mode = clientconfig.Mode(strings.ToLower(strings.TrimSpace(*mode)))
	cfg.StateDir = *stateDir
	cfg.Bypass = []string{"localhost", "127.0.0.0/8", "::1/128"}
	if err := clientconfig.Save(*configPath, cfg); err != nil {
		exitErr(err)
	}

	fmt.Printf("config saved: %s\n", *configPath)
	fmt.Println("next: chameleon doctor --config", *configPath)
}

func runConnect(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "config path")
	if err := fs.Parse(args); err != nil {
		exitErr(err)
	}

	cfg, err := clientconfig.Load(*configPath)
	if err != nil {
		exitErr(err)
	}
	app, err := clientapp.New(cfg)
	if err != nil {
		exitErr(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Chameleon %s mode=%s SOCKS5=%s\n", buildversion.Current, cfg.Mode, cfg.Listen)
	if err := app.ListenAndServe(ctx); err != nil {
		exitErr(err)
	}
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "config path")
	if err := fs.Parse(args); err != nil {
		exitErr(err)
	}

	cfg, err := clientconfig.Load(*configPath)
	if err != nil {
		exitErr(err)
	}
	conn, err := net.DialTimeout("tcp", cfg.Listen, 700*time.Millisecond)
	if err != nil {
		fmt.Printf("stopped: SOCKS5 is not reachable at %s\n", cfg.Listen)
		os.Exit(1)
	}
	_ = conn.Close()
	fmt.Printf("running: SOCKS5 is listening at %s (mode=%s)\n", cfg.Listen, cfg.Mode)
}

func runShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "config path")
	if err := fs.Parse(args); err != nil {
		exitErr(err)
	}

	cfg, err := clientconfig.Load(*configPath)
	if err != nil {
		exitErr(err)
	}
	cfg.PSK = redact(cfg.PSK)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		exitErr(err)
	}
	fmt.Println(string(data))
}

func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "config path")
	jsonOutput := fs.Bool("json", false, "machine-readable JSON")
	samples := fs.Int("samples", 2, "samples per configured carrier")
	if err := fs.Parse(args); err != nil {
		exitErr(err)
	}

	cfg, err := clientconfig.Load(*configPath)
	if err != nil {
		exitErr(err)
	}
	network, networkErr := networkctx.Detect()
	report := doctorReport{
		Version: buildversion.Current,
		Config:  *configPath,
		Network: network,
	}
	if networkErr != nil {
		report.Checks = append(report.Checks, doctorCheck{
			Name: "network-context", Error: networkErr.Error(),
		})
	} else {
		report.Checks = append(report.Checks, doctorCheck{Name: "network-context", OK: true})
	}

	tlsCfg := tunnel.TLSClientConfig{
		ServerName:   cfg.TLSServerName,
		PinnedSHA256: cfg.TLSFingerprint,
	}
	ctx := context.Background()
	if cfg.QUICServer != "" {
		report.Checks = append(report.Checks, measureCheck(ctx, "quic", cfg.QUICServer, *samples, func(ctx context.Context) error {
			_, err := tunnel.ProbeQUICContext(ctx, cfg.QUICServer, cfg.PSK, 5*time.Second, tlsCfg)
			return err
		}))
	}
	if cfg.TLSServer != "" {
		report.Checks = append(report.Checks, measureCheck(ctx, "tls", cfg.TLSServer, *samples, func(ctx context.Context) error {
			_, err := tunnel.ProbeTLSContext(ctx, cfg.TLSServer, cfg.PSK, 5*time.Second, tlsCfg)
			return err
		}))
	}
	if cfg.TCPServer != "" {
		report.Checks = append(report.Checks, measureCheck(ctx, "tcp", cfg.TCPServer, *samples, func(ctx context.Context) error {
			_, err := tunnel.ProbeTCPContext(ctx, cfg.TCPServer, cfg.PSK, 5*time.Second)
			return err
		}))
	}

	if *jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			exitErr(err)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("Chameleon %s doctor\n", report.Version)
		fmt.Printf("network: %s link=%s ipv4=%t ipv6=%t\n", network.ID, network.Link, network.IPv4, network.IPv6)
		for _, check := range report.Checks {
			if check.OK {
				if check.Latency > 0 {
					fmt.Printf("OK   %-16s %-24s %s\n", check.Name, check.Target, check.Latency.Round(time.Millisecond))
				} else {
					fmt.Printf("OK   %s\n", check.Name)
				}
			} else {
				fmt.Printf("FAIL %-16s %-24s %s\n", check.Name, check.Target, check.Error)
			}
		}
	}

	for _, check := range report.Checks {
		if !check.OK {
			os.Exit(1)
		}
	}
}

func measureCheck(
	ctx context.Context,
	name string,
	target string,
	samples int,
	run func(context.Context) error,
) doctorCheck {
	summary := probe.Measure(ctx, samples, 100*time.Millisecond, run)
	check := doctorCheck{
		Name:    name,
		Target:  target,
		OK:      summary.Successes > 0,
		Latency: summary.AvgLatency,
	}
	if summary.Successes == 0 {
		check.Error = fmt.Sprintf("%d/%d probes failed", summary.Failures, summary.Attempts)
	}
	return check
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "chameleon.json"
	}
	return filepath.Join(dir, "chameleon", "config.json")
}

func defaultStateDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ".chameleon-state"
	}
	return filepath.Join(dir, "chameleon", "state")
}

func redact(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "…" + value[len(value)-4:]
}

func exitErr(err error) {
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(2)
}
