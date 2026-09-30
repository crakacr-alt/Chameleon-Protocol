package tunnel

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
)

type probeRecordingConn struct {
	net.Conn
	mu     sync.Mutex
	writes []int
}

func (c *probeRecordingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes = append(c.writes, len(p))
	c.mu.Unlock()
	return c.Conn.Write(p)
}

func (c *probeRecordingConn) writeSizes() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.writes...)
}

func testTLSCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
	return cert, der
}

func TestTLSFrontTunnelEndToEndWithPinnedCertificate(t *testing.T) {
	cert, der := testTLSCertificate(t)
	base, err := NewServer(ServerConfig{
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
	front, err := NewTLSFront(base, TLSFrontConfig{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	})
	if err != nil {
		t.Fatal(err)
	}

	clientSide, serverSide := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- front.HandleConn(context.Background(), serverSide)
	}()

	sum := sha256.Sum256(der)
	conn, err := DialTLSContextWithDialer(
		context.Background(),
		"localhost:443",
		"example.com:443",
		"secret",
		time.Second,
		TLSClientConfig{
			ServerName:   "localhost",
			PinnedSHA256: hex.EncodeToString(sum[:]),
		},
		func(context.Context, string) (net.Conn, error) {
			return clientSide, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 5)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "hello" {
		t.Fatalf("unexpected reply %q", reply)
	}

	_ = conn.Close()
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("TLS tunnel handler did not stop")
	}
}

func TestProbeTLSWithDialerCanSplitFirstClientHello(t *testing.T) {
	cert, der := testTLSCertificate(t)
	base, err := NewServer(ServerConfig{
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
	front, err := NewTLSFront(base, TLSFrontConfig{TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}})
	if err != nil {
		t.Fatal(err)
	}

	clientSide, serverSide := net.Pipe()
	serverDone := make(chan error, 1)
	go func() { serverDone <- front.HandleConn(context.Background(), serverSide) }()

	sum := sha256.Sum256(der)
	wrapped := &probeRecordingConn{Conn: clientSide}
	_, err = ProbeTLSWithDialer(
		context.Background(),
		"localhost:443",
		"secret",
		time.Second,
		TLSClientConfig{ServerName: "localhost", PinnedSHA256: hex.EncodeToString(sum[:])},
		func(context.Context, string) (net.Conn, error) {
			return dpi.NewFirstWriteConn(wrapped, dpi.DefaultStrategies()[1]), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("TLS probe server did not stop")
	}

	writes := wrapped.writeSizes()
	if len(writes) < 2 || writes[0] != 1 {
		t.Fatalf("expected split-early ClientHello writes, got %v", writes)
	}
}

func TestTLSFrontServesHTTPDecoy(t *testing.T) {
	cert, _ := testTLSCertificate(t)
	base, err := NewServer(ServerConfig{PSK: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	front, err := NewTLSFront(base, TLSFrontConfig{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		DecoyBody: "<html>normal site</html>\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	clientSide, serverSide := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- front.HandleConn(context.Background(), serverSide)
	}()

	client := tls.Client(clientSide, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
		NextProtos:         []string{"http/1.1"},
	})
	if err := client.Handshake(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, "https://localhost/", nil)
	response, err := http.ReadResponse(bufio.NewReader(client), req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %s", response.Status)
	}
	if !strings.Contains(string(body), "normal site") {
		t.Fatalf("unexpected decoy body %q", body)
	}

	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decoy handler did not stop")
	}
}

func TestParseFingerprint(t *testing.T) {
	sum := sha256.Sum256([]byte("certificate"))
	text := strings.ToUpper(hex.EncodeToString(sum[:]))
	withColons := strings.Join(splitPairs(text), ":")
	got, err := parseFingerprint(withColons)
	if err != nil {
		t.Fatal(err)
	}
	if got != sum {
		t.Fatal("parsed fingerprint changed")
	}
}

func splitPairs(s string) []string {
	out := make([]string, 0, len(s)/2)
	for len(s) >= 2 {
		out = append(out, s[:2])
		s = s[2:]
	}
	return out
}

func TestTLSClientConfigUsesCompactCurvePreferences(t *testing.T) {
	cfg, err := buildTLSClientConfig("127.0.0.1:443", TLSClientConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CurvePreferences) != 2 || cfg.CurvePreferences[0] != tls.X25519 || cfg.CurvePreferences[1] != tls.CurveP256 {
		t.Fatalf("want compact X25519/P-256 preferences, got %v", cfg.CurvePreferences)
	}
}

func TestTLSClientHelloFitsMobileFirstFlight(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()

	cfg, err := buildTLSClientConfig("127.0.0.1:443", TLSClientConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	client := tls.Client(clientSide, cfg)
	defer client.Close()

	done := make(chan error, 1)
	go func() { done <- client.Handshake() }()

	if err := serverSide.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var header [5]byte
	if _, err := io.ReadFull(serverSide, header[:]); err != nil {
		t.Fatal(err)
	}
	if header[0] != 0x16 {
		t.Fatalf("expected TLS handshake record, got 0x%x", header[0])
	}
	if recordLen := int(binary.BigEndian.Uint16(header[3:5])); recordLen > 1200 {
		t.Fatalf("TLS ClientHello record exceeds mobile first flight: %d bytes", recordLen)
	}

	_ = serverSide.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TLS handshake did not stop")
	}
}
