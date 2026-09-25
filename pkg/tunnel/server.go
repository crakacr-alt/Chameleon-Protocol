package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// DialContextFunc lets tests and future policy layers control server egress.
type DialContextFunc func(context.Context, string, string) (net.Conn, error)

// ServerConfig configures the authenticated TCP tunnel server.
type ServerConfig struct {
	PSK              string
	HandshakeTimeout time.Duration
	DialTimeout      time.Duration
	MaxClockSkew     time.Duration
	DialContext      DialContextFunc
}

// Server accepts authenticated tunnel streams and forwards them to destinations.
type Server struct {
	cfg ServerConfig

	replayMu sync.Mutex
	seen     map[[nonceSize]byte]time.Time
}

// NewServer validates config and creates a tunnel server.
func NewServer(cfg ServerConfig) (*Server, error) {
	if stringsTrim(cfg.PSK) == "" {
		return nil, fmt.Errorf("psk must not be empty")
	}
	if cfg.HandshakeTimeout <= 0 {
		cfg.HandshakeTimeout = 8 * time.Second
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 8 * time.Second
	}
	if cfg.MaxClockSkew <= 0 {
		cfg.MaxClockSkew = 2 * time.Minute
	}
	if cfg.DialContext == nil {
		dialer := &net.Dialer{Timeout: cfg.DialTimeout}
		cfg.DialContext = dialer.DialContext
	}
	return &Server{cfg: cfg, seen: make(map[[nonceSize]byte]time.Time)}, nil
}

// Serve accepts connections until listener.Close or an accept error.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if s == nil {
		return fmt.Errorf("server is nil")
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
			_ = s.HandleConn(ctx, conn)
		}()
	}
}

// HandleConn authenticates one client and proxies one destination stream.
func (s *Server) HandleConn(ctx context.Context, conn net.Conn) error {
	if s == nil || conn == nil {
		return fmt.Errorf("invalid tunnel connection")
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(s.cfg.HandshakeTimeout)); err != nil {
		return err
	}
	hello, err := readClientHello(conn, s.cfg.PSK, time.Now(), s.cfg.MaxClockSkew)
	if err != nil {
		return err
	}
	if !s.acceptNonce(hello.Nonce, time.Now()) {
		return fmt.Errorf("replayed tunnel hello")
	}

	cipher, err := deriveCipher(s.cfg.PSK, hello.Nonce)
	if err != nil {
		return err
	}
	secure := newSecureConn(conn, cipher)

	upstreamCtx, cancel := context.WithTimeout(ctx, s.cfg.DialTimeout)
	defer cancel()
	upstream, err := s.cfg.DialContext(upstreamCtx, "tcp", hello.Destination)
	if err != nil {
		message := append([]byte{1}, []byte("remote dial failed")...)
		_, _ = secure.Write(message)
		return fmt.Errorf("dial destination: %w", err)
	}
	defer upstream.Close()

	if _, err := secure.Write([]byte{0}); err != nil {
		return fmt.Errorf("send tunnel ready status: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return err
	}

	errCh := make(chan error, 2)
	go func() {
		_, copyErr := io.Copy(upstream, secure)
		errCh <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(secure, upstream)
		errCh <- copyErr
	}()

	first := <-errCh
	_ = upstream.Close()
	_ = conn.Close()
	second := <-errCh

	if first != nil && !isClosedError(first) {
		return first
	}
	if second != nil && !isClosedError(second) {
		return second
	}
	return nil
}

func (s *Server) acceptNonce(nonce [nonceSize]byte, now time.Time) bool {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()

	for value, seenAt := range s.seen {
		if now.Sub(seenAt) > s.cfg.MaxClockSkew {
			delete(s.seen, value)
		}
	}
	if _, ok := s.seen[nonce]; ok {
		return false
	}
	s.seen[nonce] = now
	return true
}

func isClosedError(err error) bool {
	return err == nil || errors.Is(err, net.ErrClosed)
}
