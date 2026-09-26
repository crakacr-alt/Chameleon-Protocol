package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
)

func main() {
	listen := flag.String("listen", firstNonEmpty(os.Getenv("CHAMELEON_LISTEN"), ":9443"), "TCP address for Chameleon tunnel")
	psk := flag.String("psk", "", "required pre-shared key for tunnel authentication")
	handshakeTimeout := flag.Duration("handshake-timeout", 8*time.Second, "client handshake timeout")
	dialTimeout := flag.Duration("dial-timeout", 8*time.Second, "destination dial timeout")
	allowPrivate := flag.Bool("allow-private", false, "allow tunnel egress to private/loopback server networks")
	tlsCert := flag.String("tls-cert", "", "optional PEM certificate path; enables TLS front")
	tlsKey := flag.String("tls-key", "", "optional PEM private-key path; enables TLS front")
	decoyFile := flag.String("decoy-file", os.Getenv("CHAMELEON_DECOY_FILE"), "optional HTML file returned to ordinary HTTPS GET probes")
	flag.Parse()

	tunnelPSK := *psk
	if tunnelPSK == "" {
		tunnelPSK = os.Getenv("CHAMELEON_TUNNEL_PSK")
	}
	if tunnelPSK == "" {
		fmt.Fprintln(os.Stderr, "error: --psk or CHAMELEON_TUNNEL_PSK is required")
		os.Exit(2)
	}

	certPath := firstNonEmpty(*tlsCert, os.Getenv("CHAMELEON_TLS_CERT"))
	keyPath := firstNonEmpty(*tlsKey, os.Getenv("CHAMELEON_TLS_KEY"))
	if (certPath == "") != (keyPath == "") {
		fmt.Fprintln(os.Stderr, "error: TLS requires both certificate and private key")
		os.Exit(2)
	}

	server, err := tunnel.NewServer(tunnel.ServerConfig{
		PSK:                      tunnelPSK,
		HandshakeTimeout:         *handshakeTimeout,
		DialTimeout:              *dialTimeout,
		AllowPrivateDestinations: *allowPrivate,
	})
	if err != nil {
		panic(err)
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

	if certPath == "" {
		fmt.Printf("Chameleon raw TCP tunnel listening on %s\n", listener.Addr())
		err = server.Serve(ctx, listener)
		if err != nil && ctx.Err() == nil {
			panic(err)
		}
		return
	}

	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		panic(fmt.Errorf("load TLS certificate: %w", err))
	}

	decoyBody := ""
	if *decoyFile != "" {
		data, readErr := os.ReadFile(*decoyFile)
		if readErr != nil {
			panic(fmt.Errorf("read decoy file: %w", readErr))
		}
		decoyBody = string(data)
	}

	front, err := tunnel.NewTLSFront(server, tunnel.TLSFrontConfig{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{certificate},
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"http/1.1"},
		},
		HandshakeTimeout: *handshakeTimeout,
		DecoyBody:        decoyBody,
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Chameleon TLS tunnel listening on %s\n", listener.Addr())
	err = front.Serve(ctx, listener)
	if err != nil && ctx.Err() == nil {
		panic(err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
