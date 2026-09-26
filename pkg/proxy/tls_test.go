package proxy

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
	"sync"
	"testing"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
)

type writeRecordingConn struct {
	net.Conn
	mu      sync.Mutex
	lengths []int
}

func (c *writeRecordingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.lengths = append(c.lengths, len(p))
	c.mu.Unlock()
	return c.Conn.Write(p)
}

func (c *writeRecordingConn) firstWriteLength() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.lengths) == 0 {
		return 0
	}
	return c.lengths[0]
}

func proxyTestCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(7),
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

func TestAdaptiveTLSCarrierSplitsRealClientHello(t *testing.T) {
	cert, der := proxyTestCertificate(t)
	base, err := tunnel.NewServer(tunnel.ServerConfig{
		PSK:              "secret",
		HandshakeTimeout: time.Second,
		DialTimeout:      time.Second,
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
	front, err := tunnel.NewTLSFront(base, tunnel.TLSFrontConfig{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	})
	if err != nil {
		t.Fatal(err)
	}

	carrierEngine, err := carrier.NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	dpiEngine, err := dpi.NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	p, err := planner.New(carrierEngine, dpiEngine)
	if err != nil {
		t.Fatal(err)
	}

	networkID := "tls-test-net"
	endpoint := "localhost:443"
	// One direct-strategy failure makes split-early the next DPI choice for
	// the visible TLS endpoint.
	if err := dpiEngine.Observe(dpi.Observation{
		Context: dpi.Context{
			NetworkID:    networkID,
			Destination:  endpoint,
			TrafficClass: "web",
		},
		Strategy: "direct",
		Success:  false,
		Failure:  "simulated DPI reset",
		At:       time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	clientSide, serverSide := net.Pipe()
	recording := &writeRecordingConn{Conn: clientSide}
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- front.HandleConn(context.Background(), serverSide)
	}()

	sum := sha256.Sum256(der)
	dialer := &AdaptiveDialer{
		Planner:       p,
		Carriers:      carrier.WithTLS(nil, endpoint),
		DPIStrategies: dpi.DefaultStrategies(),
		PSK:           "secret",
		TLSConfig: tunnel.TLSClientConfig{
			ServerName:   "localhost",
			PinnedSHA256: hex.EncodeToString(sum[:]),
		},
		Timeout: time.Second,
		Network: func() (networkctx.Context, error) {
			return networkctx.Context{ID: networkID}, nil
		},
		DirectDial: func(context.Context, string) (net.Conn, error) {
			return recording, nil
		},
	}

	conn, err := dialer.DialContext(context.Background(), "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if got := recording.firstWriteLength(); got != 1 {
		t.Fatalf("expected split-early to split the TLS ClientHello at byte 1, first write=%d", got)
	}

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 5)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "hello" {
		t.Fatalf("unexpected tunnel reply %q", reply)
	}
	_ = conn.Close()

	dpiStats := dpiEngine.Snapshot(dpi.Context{
		NetworkID:    networkID,
		Destination:  endpoint,
		TrafficClass: "web",
	})["split-early"]
	if dpiStats.Successes != 1 {
		t.Fatalf("TLS handshake should prove first-hop DPI strategy, got %+v", dpiStats)
	}

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("TLS front did not stop")
	}
}
