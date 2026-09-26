package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
	adaptiveproxy "github.com/crakacr-alt/Chameleon-Protocol/pkg/proxy"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/socks5"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
	buildversion "github.com/crakacr-alt/Chameleon-Protocol/pkg/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print Chameleon version and exit")
	listen := flag.String("listen", "127.0.0.1:1080", "local SOCKS5 listen address")
	chameleonTCP := flag.String("chameleon-tcp", "", "optional raw Chameleon TCP tunnel endpoint")
	chameleonTLS := flag.String("chameleon-tls", "", "optional TLS-fronted Chameleon tunnel endpoint")
	chameleonQUIC := flag.String("chameleon-quic", "", "optional UDP/QUIC Chameleon tunnel endpoint")
	udpMode := flag.String("udp-mode", "auto", "SOCKS5 UDP mode: auto, direct or quic")
	tlsServerName := flag.String("tls-server-name", "", "TLS server name/SNI for --chameleon-tls")
	tlsInsecure := flag.Bool("tls-insecure", false, "skip normal TLS certificate verification (not recommended)")
	tlsFingerprint := flag.String("tls-fingerprint", "", "optional pinned TLS certificate SHA-256 fingerprint")
	relaySOCKS := flag.String("relay-socks", "", "optional existing SOCKS5 relay endpoint")
	allowRemote := flag.Bool("allow-remote-socks", false, "allow SOCKS listener on non-loopback addresses")
	psk := flag.String("psk", "", "PSK for --chameleon-tcp")
	stateDir := flag.String("state-dir", defaultStateDir(), "adaptive state directory")
	timeout := flag.Duration("timeout", 8*time.Second, "outbound connection timeout")
	directCooldown := flag.Duration("direct-cooldown", 10*time.Minute, "temporarily skip direct after all DPI strategies fail")
	failureWindow := flag.Duration("failure-window", 12*time.Second, "first-response window used for automatic DPI failure learning")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildversion.Current)
		return
	}

	tunnelPSK := *psk
	if tunnelPSK == "" {
		tunnelPSK = os.Getenv("CHAMELEON_TUNNEL_PSK")
	}
	if (*chameleonTCP != "" || *chameleonTLS != "" || *chameleonQUIC != "") && tunnelPSK == "" {
		fmt.Fprintln(os.Stderr, "error: --psk or CHAMELEON_TUNNEL_PSK is required for Chameleon tunnel carriers")
		os.Exit(2)
	}
	if !*allowRemote && !isLoopbackListen(*listen) {
		fmt.Fprintln(os.Stderr, "error: non-loopback SOCKS listen requires --allow-remote-socks")
		os.Exit(2)
	}

	carrierStore := ""
	dpiStore := ""
	if *stateDir != "" {
		carrierStore = filepath.Join(*stateDir, "carriers.json")
		dpiStore = filepath.Join(*stateDir, "dpi.json")
	}

	carrierEngine, err := carrier.NewEngine(carrierStore)
	if err != nil {
		panic(err)
	}
	dpiEngine, err := dpi.NewEngine(dpiStore)
	if err != nil {
		panic(err)
	}
	p, err := planner.New(carrierEngine, dpiEngine)
	if err != nil {
		panic(err)
	}

	carriers := carrier.WithQUIC(
		carrier.WithTLS(
			carrier.Defaults("", *chameleonTCP, *relaySOCKS),
			*chameleonTLS,
		),
		*chameleonQUIC,
	)

	dialer := &adaptiveproxy.AdaptiveDialer{
		Planner:       p,
		Carriers:      carriers,
		DPIStrategies: dpi.DefaultStrategies(),
		PSK:           tunnelPSK,
		TLSConfig: tunnel.TLSClientConfig{
			ServerName:         *tlsServerName,
			InsecureSkipVerify: *tlsInsecure,
			PinnedSHA256:       *tlsFingerprint,
		},
		Timeout:                  *timeout,
		DirectCooldown:           *directCooldown,
		ApplicationFailureWindow: *failureWindow,
		Network:                  networkctx.Detect,
	}

	tlsCfg := tunnel.TLSClientConfig{
		ServerName:         *tlsServerName,
		InsecureSkipVerify: *tlsInsecure,
		PinnedSHA256:       *tlsFingerprint,
	}

	openUDP := func(ctx context.Context) (socks5.UDPAssociation, error) {
		switch strings.ToLower(strings.TrimSpace(*udpMode)) {
		case "direct":
			return socks5.NewDirectUDPAssociation(ctx, *timeout), nil
		case "quic":
			if *chameleonQUIC == "" {
				return nil, fmt.Errorf("--udp-mode=quic requires --chameleon-quic")
			}
			return tunnel.DialQUICDatagramSession(ctx, *chameleonQUIC, tunnelPSK, *timeout, tlsCfg)
		case "auto":
			if *chameleonQUIC != "" {
				return tunnel.DialQUICDatagramSession(ctx, *chameleonQUIC, tunnelPSK, *timeout, tlsCfg)
			}
			return socks5.NewDirectUDPAssociation(ctx, *timeout), nil
		default:
			return nil, fmt.Errorf("unknown --udp-mode %q", *udpMode)
		}
	}

	server := &socks5.Server{
		Dial:    dialer.DialContext,
		OpenUDP: openUDP,
		OnSession: func(result socks5.SessionResult) {
			if result.Err != nil {
				fmt.Printf("session %s ended: up=%d down=%d duration=%s err=%v\n",
					result.Destination, result.BytesUp, result.BytesDown, result.Duration.Round(time.Millisecond), result.Err)
				return
			}
			fmt.Printf("session %s: up=%d down=%d duration=%s\n",
				result.Destination, result.BytesUp, result.BytesDown, result.Duration.Round(time.Millisecond))
		},
	}

	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	fmt.Printf("Chameleon SOCKS5 proxy listening on %s\n", listener.Addr())
	if *chameleonQUIC != "" {
		fmt.Printf("Chameleon QUIC fallback: %s\n", *chameleonQUIC)
	}
	if *chameleonTLS != "" {
		fmt.Printf("Chameleon TLS fallback: %s\n", *chameleonTLS)
	}
	if *chameleonTCP != "" {
		fmt.Printf("Chameleon raw TCP fallback: %s\n", *chameleonTCP)
	}
	if *relaySOCKS != "" {
		fmt.Printf("external SOCKS relay fallback: %s\n", *relaySOCKS)
	}

	err = server.Serve(ctx, listener)
	if err != nil && ctx.Err() == nil {
		panic(err)
	}
}

func defaultStateDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "chameleon")
}

func isLoopbackListen(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
