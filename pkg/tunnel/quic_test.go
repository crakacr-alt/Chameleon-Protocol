package tunnel

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func quicTestCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}, der
}

func TestQUICAuthenticatedTunnelEndToEnd(t *testing.T) {
	cert, der := quicTestCertificate(t)

	server, err := NewServer(ServerConfig{
		PSK:                      "secret",
		HandshakeTimeout:         2 * time.Second,
		DialTimeout:              2 * time.Second,
		AllowPrivateDestinations: true,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			upstreamServer, upstreamEcho := net.Pipe()
			go func() {
				_, _ = io.Copy(upstreamEcho, upstreamEcho)
				_ = upstreamEcho.Close()
			}()
			return upstreamServer, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	listener, err := NewQUICListener(
		"127.0.0.1:0",
		&tls.Config{Certificates: []tls.Certificate{cert}},
		server,
		QUICConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- listener.Serve(ctx)
	}()

	sum := sha256.Sum256(der)
	conn, err := DialQUICContext(
		context.Background(),
		listener.Addr().String(),
		"127.0.0.1:443",
		"secret",
		3*time.Second,
		TLSClientConfig{
			ServerName:   "localhost",
			PinnedSHA256: hex.EncodeToString(sum[:]),
		},
		QUICConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Write([]byte("hello-quic")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, len("hello-quic"))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "hello-quic" {
		t.Fatalf("unexpected QUIC tunnel reply %q", reply)
	}

	_ = conn.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("QUIC listener did not stop")
	}
}

func TestQUICRejectsWrongCertificatePin(t *testing.T) {
	cert, _ := quicTestCertificate(t)
	server, err := NewServer(ServerConfig{PSK: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := NewQUICListener(
		"127.0.0.1:0",
		&tls.Config{Certificates: []tls.Certificate{cert}},
		server,
		QUICConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = listener.Serve(ctx) }()

	_, err = DialQUICContext(
		context.Background(),
		listener.Addr().String(),
		"example.com:443",
		"secret",
		time.Second,
		TLSClientConfig{
			ServerName:   "localhost",
			PinnedSHA256: string(make([]byte, 64)),
		},
		QUICConfig{},
	)
	if err == nil {
		t.Fatal("wrong TLS pin must fail")
	}
}
