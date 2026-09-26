package tunnel

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestQUICDatagramSessionEndToEnd(t *testing.T) {
	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()

	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := echo.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = echo.WriteToUDP(buf[:n], addr)
		}
	}()

	cert, der := quicTestCertificate(t)
	server, err := NewServer(ServerConfig{
		PSK:                      "datagram-secret",
		HandshakeTimeout:         2 * time.Second,
		DialTimeout:              2 * time.Second,
		AllowPrivateDestinations: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	listener, err := NewQUICListener(
		"127.0.0.1:0",
		&tls.Config{Certificates: []tls.Certificate{cert}},
		server,
		QUICConfig{EnableDatagrams: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() { _ = listener.Serve(serverCtx) }()

	sum := sha256.Sum256(der)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	session, err := DialQUICDatagramSession(
		ctx,
		listener.Addr().String(),
		"datagram-secret",
		2*time.Second,
		TLSClientConfig{
			ServerName:   "localhost",
			PinnedSHA256: hex.EncodeToString(sum[:]),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	destination := echo.LocalAddr().String()
	if err := session.Send(ctx, destination, []byte("hello-udp")); err != nil {
		t.Fatal(err)
	}
	gotDestination, payload, err := session.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if gotDestination != destination {
		t.Fatalf("want destination %q, got %q", destination, gotDestination)
	}
	if string(payload) != "hello-udp" {
		t.Fatalf("unexpected payload %q", payload)
	}
}

func TestQUICDatagramRejectsWrongPSK(t *testing.T) {
	cert, der := quicTestCertificate(t)
	server, err := NewServer(ServerConfig{PSK: "right-secret"})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := NewQUICListener(
		"127.0.0.1:0",
		&tls.Config{Certificates: []tls.Certificate{cert}},
		server,
		QUICConfig{EnableDatagrams: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() { _ = listener.Serve(serverCtx) }()

	sum := sha256.Sum256(der)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = DialQUICDatagramSession(
		ctx,
		listener.Addr().String(),
		"wrong-secret",
		time.Second,
		TLSClientConfig{
			ServerName:   "localhost",
			PinnedSHA256: hex.EncodeToString(sum[:]),
		},
	)
	if err == nil {
		t.Fatal("wrong PSK must reject QUIC datagram session")
	}
}

func TestQUICDatagramSizeLimitIsExplicit(t *testing.T) {
	_, err := encodeDatagramFrame("example.com:53", []byte(strings.Repeat("x", maxDatagramPlain)))
	if err == nil {
		t.Fatal("oversized datagram must fail")
	}
	if !errors.Is(err, ErrDatagramTooLarge) {
		t.Fatalf("want ErrDatagramTooLarge, got %v", err)
	}
}

func TestDatagramFrameRoundTrip(t *testing.T) {
	frame, err := encodeDatagramFrame("example.com:53", []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	destination, payload, err := decodeDatagramFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if destination != "example.com:53" || string(payload) != "payload" {
		t.Fatalf("unexpected datagram frame: %s %q", destination, payload)
	}
}
