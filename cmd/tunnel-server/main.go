package main

import (
	"context"
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
	listen := flag.String("listen", ":9443", "TCP address for Chameleon tunnel")
	psk := flag.String("psk", "", "required pre-shared key for tunnel authentication")
	handshakeTimeout := flag.Duration("handshake-timeout", 8*time.Second, "client handshake timeout")
	dialTimeout := flag.Duration("dial-timeout", 8*time.Second, "destination dial timeout")
	flag.Parse()

	if *psk == "" {
		fmt.Fprintln(os.Stderr, "error: --psk is required")
		os.Exit(2)
	}

	server, err := tunnel.NewServer(tunnel.ServerConfig{
		PSK:              *psk,
		HandshakeTimeout: *handshakeTimeout,
		DialTimeout:      *dialTimeout,
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

	fmt.Printf("Chameleon TCP tunnel listening on %s\n", listener.Addr())
	err = server.Serve(ctx, listener)
	if err != nil && ctx.Err() == nil {
		panic(err)
	}
}
