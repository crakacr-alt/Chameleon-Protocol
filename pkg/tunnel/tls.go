package tunnel

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TLSClientConfig controls the standard TLS layer in front of the Chameleon
// tunnel. By default normal certificate verification is required.
type TLSClientConfig struct {
	ServerName         string
	InsecureSkipVerify bool
	PinnedSHA256       string
	RootCAs            *x509.CertPool
}

// DialTLSContext opens a normal TLS connection first and then runs the
// authenticated Chameleon tunnel handshake inside that encrypted stream.
func DialTLSContext(
	ctx context.Context,
	serverAddress string,
	destination string,
	psk string,
	timeout time.Duration,
	cfg TLSClientConfig,
) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	return DialTLSContextWithDialer(
		ctx,
		serverAddress,
		destination,
		psk,
		timeout,
		cfg,
		func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		},
	)
}

// DialTLSContextWithDialer lets the caller wrap the raw TCP first hop.
// The adaptive proxy uses this to apply a learned split strategy to the
// real TLS ClientHello before crypto/tls writes it to the network.
func DialTLSContextWithDialer(
	ctx context.Context,
	serverAddress string,
	destination string,
	psk string,
	timeout time.Duration,
	cfg TLSClientConfig,
	dial RawDialFunc,
) (net.Conn, error) {
	if stringsTrim(serverAddress) == "" {
		return nil, fmt.Errorf("server address must not be empty")
	}
	if dial == nil {
		return nil, fmt.Errorf("raw dial function is nil")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	raw, err := dial(ctx, serverAddress)
	if err != nil {
		return nil, fmt.Errorf("dial TLS tunnel server: %w", err)
	}

	tlsConfig, err := buildTLSClientConfig(serverAddress, cfg)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}

	tlsConn := tls.Client(raw, tlsConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}
	if err := tlsConn.SetDeadline(time.Time{}); err != nil {
		_ = tlsConn.Close()
		return nil, err
	}

	secure, err := clientHandshake(tlsConn, destination, psk, timeout)
	if err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	return secure, nil
}

func buildTLSClientConfig(serverAddress string, cfg TLSClientConfig) (*tls.Config, error) {
	serverName := strings.TrimSpace(cfg.ServerName)
	if serverName == "" {
		host, _, err := net.SplitHostPort(serverAddress)
		if err != nil {
			return nil, fmt.Errorf("parse TLS server address: %w", err)
		}
		serverName = host
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         serverName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		RootCAs:            cfg.RootCAs,
		NextProtos:         []string{"http/1.1"},
	}

	if strings.TrimSpace(cfg.PinnedSHA256) != "" {
		want, err := parseFingerprint(cfg.PinnedSHA256)
		if err != nil {
			return nil, err
		}
		// Pinning performs its own certificate identity check. This is useful
		// for a private/self-signed certificate while still preventing MITM.
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return fmt.Errorf("TLS peer did not provide a certificate")
			}
			got := sha256.Sum256(state.PeerCertificates[0].Raw)
			if got != want {
				return fmt.Errorf("TLS certificate fingerprint mismatch")
			}
			return nil
		}
	}

	return tlsConfig, nil
}

func parseFingerprint(value string) ([sha256.Size]byte, error) {
	var out [sha256.Size]byte
	clean := strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(value))
	raw, err := hex.DecodeString(clean)
	if err != nil || len(raw) != sha256.Size {
		return out, fmt.Errorf("TLS fingerprint must be a SHA-256 hex digest")
	}
	copy(out[:], raw)
	return out, nil
}

// TLSFrontConfig configures a normal HTTPS-looking front for a Tunnel Server.
type TLSFrontConfig struct {
	TLSConfig        *tls.Config
	HandshakeTimeout time.Duration
	DecoyBody        string
}

// TLSFront accepts standard TLS, serves a small HTTP response to ordinary HTTP
// probes, and forwards non-HTTP application data to the authenticated tunnel.
type TLSFront struct {
	tunnel *Server
	cfg    TLSFrontConfig
}

// NewTLSFront creates a TLS front around an existing tunnel server.
func NewTLSFront(server *Server, cfg TLSFrontConfig) (*TLSFront, error) {
	if server == nil {
		return nil, fmt.Errorf("tunnel server is nil")
	}
	if cfg.TLSConfig == nil || len(cfg.TLSConfig.Certificates) == 0 {
		return nil, fmt.Errorf("TLS certificate is required")
	}
	if cfg.HandshakeTimeout <= 0 {
		cfg.HandshakeTimeout = 8 * time.Second
	}
	if cfg.DecoyBody == "" {
		cfg.DecoyBody = "<!doctype html><html><head><title>Welcome</title></head><body><h1>Welcome</h1></body></html>\n"
	}

	tlsCfg := cfg.TLSConfig.Clone()
	if tlsCfg.MinVersion == 0 {
		tlsCfg.MinVersion = tls.VersionTLS12
	}
	if len(tlsCfg.NextProtos) == 0 {
		tlsCfg.NextProtos = []string{"http/1.1"}
	}
	cfg.TLSConfig = tlsCfg

	return &TLSFront{tunnel: server, cfg: cfg}, nil
}

// Serve accepts TLS connections until listener.Close or context cancellation.
func (f *TLSFront) Serve(ctx context.Context, listener net.Listener) error {
	if f == nil {
		return fmt.Errorf("TLS front is nil")
	}
	if listener == nil {
		return fmt.Errorf("listener is nil")
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return err
			}
		}
		go func() {
			_ = f.HandleConn(ctx, conn)
		}()
	}
}

// HandleConn performs TLS first. HTTP-looking traffic receives the decoy;
// everything else is handed to the authenticated tunnel parser.
func (f *TLSFront) HandleConn(ctx context.Context, raw net.Conn) error {
	if f == nil || raw == nil {
		return fmt.Errorf("invalid TLS front connection")
	}
	defer raw.Close()

	tlsConn := tls.Server(raw, f.cfg.TLSConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(f.cfg.HandshakeTimeout)); err != nil {
		return err
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return err
	}

	reader := bufio.NewReader(tlsConn)
	prefix, err := reader.Peek(4)
	if err != nil {
		return err
	}
	if looksLikeHTTP(prefix) {
		return f.serveDecoy(tlsConn, reader)
	}

	_ = tlsConn.SetDeadline(time.Time{})
	buffered := &readerConn{Conn: tlsConn, reader: reader}
	return f.tunnel.HandleConn(ctx, buffered)
}

func (f *TLSFront) serveDecoy(conn net.Conn, reader *bufio.Reader) error {
	req, err := http.ReadRequest(reader)
	if err != nil {
		return writeHTTPResponse(conn, http.StatusBadRequest, "Bad Request\n")
	}
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return writeHTTPResponse(conn, http.StatusOK, f.cfg.DecoyBody)
}

func writeHTTPResponse(conn net.Conn, status int, body string) error {
	statusText := http.StatusText(status)
	if statusText == "" {
		statusText = "OK"
	}
	header := "HTTP/1.1 " + strconv.Itoa(status) + " " + statusText + "\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n" +
		"Cache-Control: no-store\r\n" +
		"Connection: close\r\n\r\n"
	if err := writeFull(conn, []byte(header)); err != nil {
		return err
	}
	return writeFull(conn, []byte(body))
}

func looksLikeHTTP(prefix []byte) bool {
	if len(prefix) < 4 {
		return false
	}
	p := string(prefix[:4])
	switch p {
	case "GET ", "POST", "HEAD", "OPTI", "CONN", "PUT ", "PATC", "DELE":
		return true
	default:
		return false
	}
}

type readerConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *readerConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
