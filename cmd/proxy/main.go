package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
	adaptiveproxy "github.com/crakacr-alt/Chameleon-Protocol/pkg/proxy"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/socks5"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:1080", "local SOCKS5 listen address")
	chameleonTCP := flag.String("chameleon-tcp", "", "optional Chameleon TCP tunnel endpoint")
	relaySOCKS := flag.String("relay-socks", "", "optional existing SOCKS5 relay endpoint")
	allowRemote := flag.Bool("allow-remote-socks", false, "allow SOCKS listener on non-loopback addresses")
	psk := flag.String("psk", "", "PSK for --chameleon-tcp")
	stateDir := flag.String("state-dir", defaultStateDir(), "adaptive state directory")
	timeout := flag.Duration("timeout", 8*time.Second, "outbound connection timeout")
	flag.Parse()

	if *chameleonTCP != "" && *psk == "" {
		fmt.Fprintln(os.Stderr, "error: --psk is required when --chameleon-tcp is set")
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

	dialer := &adaptiveproxy.AdaptiveDialer{
		Planner:       p,
		Carriers:      carrier.Defaults("", *chameleonTCP, *relaySOCKS),
		DPIStrategies: dpi.DefaultStrategies(),
		PSK:           *psk,
		Timeout:       *timeout,
		Network:       networkctx.Detect,
	}

	server := &socks5.Server{
		Dial: dialer.DialContext,
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
	if *chameleonTCP != "" {
		fmt.Printf("Chameleon TCP fallback: %s\n", *chameleonTCP)
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
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
