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
	buildversion "github.com/crakacr-alt/Chameleon-Protocol/pkg/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print Chameleon version and exit")
	listen := flag.String("listen", firstNonEmpty(os.Getenv("CHAMELEON_LISTEN"), ":9443"), "TCP address for Chameleon tunnel")
	quicListen := flag.String("quic-listen", os.Getenv("CHAMELEON_QUIC_LISTEN"), "optional UDP address for QUIC carrier")
	psk := flag.String("psk", "", "required pre-shared key for tunnel authentication")
	handshakeTimeout := flag.Duration("handshake-timeout", 8*time.Second, "client handshake timeout")
	dialTimeout := flag.Duration("dial-timeout", 8*time.Second, "destination dial timeout")
	allowPrivate := flag.Bool("allow-private", false, "allow tunnel egress to private/loopback server networks")
	tlsCert := flag.String("tls-cert", "", "optional PEM certificate path; enables TLS front")
	tlsKey := flag.String("tls-key", "", "optional PEM private-key path; enables TLS front")
	decoyFile := flag.String("decoy-file", os.Getenv("CHAMELEON_DECOY_FILE"), "optional HTML file returned to ordinary HTTPS GET probes")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildversion.Current)
		return
	}

	tunnelPSK := firstNonEmpty(*psk, os.Getenv("CHAMELEON_TUNNEL_PSK"))
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
	if *quicListen != "" && certPath == "" {
		fmt.Fprintln(os.Stderr, "error: QUIC carrier requires TLS certificate and key")
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

	var certificate tls.Certificate
	if certPath != "" {
		certificate, err = tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			panic(fmt.Errorf("load TLS certificate: %w", err))
		}
	}

	decoyBody := ""
	if *decoyFile != "" {
		data, readErr := os.ReadFile(*decoyFile)
		if readErr != nil {
			panic(fmt.Errorf("read decoy file: %w", readErr))
		}
		decoyBody = string(data)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tcpListener, err := net.Listen("tcp", *listen)
	if err != nil {
		panic(err)
	}
	defer tcpListener.Close()
	go func() {
		<-ctx.Done()
		_ = tcpListener.Close()
	}()

	errCh := make(chan error, 2)
	runners := 1

	if certPath == "" {
		fmt.Printf("Chameleon raw TCP tunnel listening on %s\n", tcpListener.Addr())
		go func() {
			errCh <- server.Serve(ctx, tcpListener)
		}()
	} else {
		front, frontErr := tunnel.NewTLSFront(server, tunnel.TLSFrontConfig{
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{certificate},
				MinVersion:   tls.VersionTLS12,
				NextProtos:   []string{"http/1.1"},
			},
			HandshakeTimeout: *handshakeTimeout,
			DecoyBody:        decoyBody,
		})
		if frontErr != nil {
			panic(frontErr)
		}

		fmt.Printf("Chameleon TLS tunnel listening on %s\n", tcpListener.Addr())
		go func() {
			errCh <- front.Serve(ctx, tcpListener)
		}()
	}

	var quicListener *tunnel.QUICListener
	if *quicListen != "" {
		quicListener, err = tunnel.NewQUICListener(
			*quicListen,
			&tls.Config{Certificates: []tls.Certificate{certificate}},
			server,
			tunnel.QUICConfig{
				HandshakeTimeout: *handshakeTimeout,
			},
		)
		if err != nil {
			panic(err)
		}
		defer quicListener.Close()
		runners++

		fmt.Printf("Chameleon QUIC tunnel listening on %s/udp\n", quicListener.Addr())
		go func() {
			errCh <- quicListener.Serve(ctx)
		}()
	}

	for i := 0; i < runners; i++ {
		runErr := <-errCh
		if runErr != nil && ctx.Err() == nil {
			stop()
			panic(runErr)
		}
		if ctx.Err() != nil {
			return
		}
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
