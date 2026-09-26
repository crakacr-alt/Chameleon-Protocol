package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"
)

const quicALPN = "chameleon-quic/1"

// QUICConfig controls the UDP/QUIC carrier. The defaults are intentionally
// conservative and work for normal web, streaming and interactive traffic.
type QUICConfig struct {
	HandshakeTimeout time.Duration
	IdleTimeout      time.Duration
	KeepAlive        time.Duration
	EnableDatagrams  bool
}

func (c QUICConfig) normalize() QUICConfig {
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = 8 * time.Second
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 45 * time.Second
	}
	if c.KeepAlive <= 0 {
		c.KeepAlive = 15 * time.Second
	}
	return c
}

func (c QUICConfig) quicConfig() *quic.Config {
	c = c.normalize()
	return &quic.Config{
		HandshakeIdleTimeout: c.HandshakeTimeout,
		MaxIdleTimeout:       c.IdleTimeout,
		KeepAlivePeriod:      c.KeepAlive,
		EnableDatagrams:      c.EnableDatagrams,
	}
}

// DialQUICContext opens a real QUIC connection over UDP and then runs the
// authenticated Chameleon tunnel handshake inside a QUIC bidirectional stream.
//
// This gives Chameleon a carrier that can survive networks where TCP is slow or
// shaped differently, while keeping the existing tunnel authentication and
// destination-hiding layer unchanged.
func DialQUICContext(
	ctx context.Context,
	serverAddress string,
	destination string,
	psk string,
	timeout time.Duration,
	tlsCfg TLSClientConfig,
	quicCfg QUICConfig,
) (net.Conn, error) {
	if stringsTrim(serverAddress) == "" {
		return nil, fmt.Errorf("QUIC server address must not be empty")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	tlsConfig, err := buildTLSClientConfig(serverAddress, tlsCfg)
	if err != nil {
		return nil, err
	}
	tlsConfig = tlsConfig.Clone()
	tlsConfig.MinVersion = tls.VersionTLS13
	tlsConfig.NextProtos = []string{quicALPN}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := quic.DialAddr(dialCtx, serverAddress, tlsConfig, quicCfg.quicConfig())
	if err != nil {
		return nil, fmt.Errorf("dial QUIC tunnel server: %w", err)
	}

	stream, err := conn.OpenStreamSync(dialCtx)
	if err != nil {
		_ = conn.CloseWithError(0, "open stream failed")
		return nil, fmt.Errorf("open QUIC tunnel stream: %w", err)
	}

	streamConn := &quicStreamConn{
		Stream: stream,
		conn:   conn,
	}
	secure, err := clientHandshake(streamConn, destination, psk, timeout)
	if err != nil {
		_ = streamConn.Close()
		return nil, err
	}
	return secure, nil
}

// QUICListener owns one UDP socket and forwards one authenticated Chameleon
// stream per QUIC connection to the existing tunnel Server.
type QUICListener struct {
	listener *quic.Listener
	server   *Server
}

// NewQUICListener starts listening on UDP address using the same certificate
// identity as the TLS/TCP front.
func NewQUICListener(address string, tlsConfig *tls.Config, server *Server, cfg QUICConfig) (*QUICListener, error) {
	if stringsTrim(address) == "" {
		return nil, fmt.Errorf("QUIC listen address must not be empty")
	}
	if tlsConfig == nil || len(tlsConfig.Certificates) == 0 {
		return nil, fmt.Errorf("QUIC TLS certificate is required")
	}
	if server == nil {
		return nil, fmt.Errorf("tunnel server is nil")
	}

	tlsCopy := tlsConfig.Clone()
	tlsCopy.MinVersion = tls.VersionTLS13
	tlsCopy.NextProtos = []string{quicALPN}

	listener, err := quic.ListenAddr(address, tlsCopy, cfg.quicConfig())
	if err != nil {
		return nil, fmt.Errorf("listen QUIC: %w", err)
	}
	return &QUICListener{listener: listener, server: server}, nil
}

func (l *QUICListener) Addr() net.Addr {
	if l == nil || l.listener == nil {
		return nil
	}
	return l.listener.Addr()
}

func (l *QUICListener) Close() error {
	if l == nil || l.listener == nil {
		return nil
	}
	return l.listener.Close()
}

// Serve accepts QUIC connections until the context is canceled.
func (l *QUICListener) Serve(ctx context.Context) error {
	if l == nil || l.listener == nil || l.server == nil {
		return fmt.Errorf("QUIC listener is not initialized")
	}

	go func() {
		<-ctx.Done()
		_ = l.listener.Close()
	}()

	for {
		conn, err := l.listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		go l.handleConn(ctx, conn)
	}
}

func (l *QUICListener) handleConn(ctx context.Context, conn *quic.Conn) {
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		_ = conn.CloseWithError(1, "stream accept failed")
		return
	}

	wrapped := &quicStreamConn{
		Stream: stream,
		conn:   conn,
	}
	_ = l.server.HandleConn(ctx, wrapped)
}

// quicStreamConn adapts a QUIC stream to net.Conn so the existing encrypted
// Chameleon tunnel framing can be reused without duplicating security logic.
type quicStreamConn struct {
	*quic.Stream
	conn      *quic.Conn
	closeOnce sync.Once
}

func (c *quicStreamConn) Close() error {
	var streamErr error
	c.closeOnce.Do(func() {
		streamErr = c.Stream.Close()
		_ = c.conn.CloseWithError(0, "")
	})
	return streamErr
}

func (c *quicStreamConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *quicStreamConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

var _ net.Conn = (*quicStreamConn)(nil)
