package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/probe"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
	buildversion "github.com/crakacr-alt/Chameleon-Protocol/pkg/version"
)

type output struct {
	Version   string                   `json:"version"`
	Server    string                   `json:"server"`
	Addresses []probe.ResolvedAddress  `json:"addresses,omitempty"`
	Results   map[string]probe.Summary `json:"results"`
}

func main() {
	showVersion := flag.Bool("version", false, "print Chameleon version and exit")
	server := flag.String("server", "", "Chameleon server host:port")
	transports := flag.String("transports", "quic,tls", "comma-separated: quic,tls,tcp")
	psk := flag.String("psk", "", "tunnel PSK; CHAMELEON_TUNNEL_PSK is preferred for normal use")
	tlsServerName := flag.String("tls-server-name", "", "TLS server name/SNI")
	tlsFingerprint := flag.String("tls-fingerprint", "", "pinned server certificate SHA-256")
	tlsInsecure := flag.Bool("tls-insecure", false, "skip normal certificate verification")
	samples := flag.Int("samples", 3, "number of authenticated probe samples per transport")
	timeout := flag.Duration("timeout", 5*time.Second, "timeout for one sample")
	interval := flag.Duration("interval", 150*time.Millisecond, "delay between samples")
	jsonOutput := flag.Bool("json", false, "print machine-readable JSON")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildversion.Current)
		return
	}
	if strings.TrimSpace(*server) == "" {
		exitErr(fmt.Errorf("--server is required"))
	}

	secret := *psk
	if secret == "" {
		secret = os.Getenv("CHAMELEON_TUNNEL_PSK")
	}
	if secret == "" {
		exitErr(fmt.Errorf("--psk or CHAMELEON_TUNNEL_PSK is required"))
	}

	tlsCfg := tunnel.TLSClientConfig{
		ServerName:         *tlsServerName,
		InsecureSkipVerify: *tlsInsecure,
		PinnedSHA256:       *tlsFingerprint,
	}

	ctx := context.Background()
	addresses, _ := probe.ResolveAddressCandidates(ctx, *server, nil)
	report := output{
		Version:   buildversion.Current,
		Server:    *server,
		Addresses: addresses,
		Results:   make(map[string]probe.Summary),
	}

	for _, name := range splitTransports(*transports) {
		var run func(context.Context) error
		switch name {
		case "quic":
			run = func(ctx context.Context) error {
				_, err := tunnel.ProbeQUICContext(ctx, *server, secret, *timeout, tlsCfg)
				return err
			}
		case "tls":
			run = func(ctx context.Context) error {
				_, err := tunnel.ProbeTLSContext(ctx, *server, secret, *timeout, tlsCfg)
				return err
			}
		case "tcp":
			run = func(ctx context.Context) error {
				_, err := tunnel.ProbeTCPContext(ctx, *server, secret, *timeout)
				return err
			}
		default:
			exitErr(fmt.Errorf("unknown transport %q", name))
		}
		report.Results[name] = probe.Measure(ctx, *samples, *interval, run)
	}

	if *jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			exitErr(err)
		}
		fmt.Println(string(data))
		return
	}

	fmt.Printf("Chameleon %s first-hop probe\n", report.Version)
	fmt.Printf("server: %s\n", report.Server)
	if len(report.Addresses) > 0 {
		fmt.Print("resolved:")
		for _, address := range report.Addresses {
			fmt.Printf(" %s=%s", address.Family, address.Address)
		}
		fmt.Println()
	}
	for _, name := range splitTransports(*transports) {
		result := report.Results[name]
		fmt.Printf("%-5s success=%d/%d loss=%.0f%% avg=%s jitter=%s\n",
			name,
			result.Successes,
			result.Attempts,
			result.LossRate*100,
			result.AvgLatency.Round(time.Millisecond),
			result.Jitter.Round(time.Millisecond),
		)
	}
}

func splitTransports(value string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 3)
	for _, part := range strings.Split(value, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(2)
}
