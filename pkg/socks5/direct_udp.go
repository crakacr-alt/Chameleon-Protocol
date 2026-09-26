package socks5

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

type directUDPResponse struct {
	destination string
	payload     []byte
	err         error
}

// DirectUDPAssociation implements the same UDPAssociation contract without a
// tunnel. It is useful when the current network allows UDP directly.
//
// One connected UDP socket is cached per destination. This preserves UDP packet
// boundaries and lets response packets retain their source destination.
type DirectUDPAssociation struct {
	ctx     context.Context
	cancel  context.CancelFunc
	timeout time.Duration

	mu    sync.Mutex
	conns map[string]net.Conn
	out   chan directUDPResponse
}

// NewDirectUDPAssociation creates a direct UDP association.
func NewDirectUDPAssociation(ctx context.Context, timeout time.Duration) *DirectUDPAssociation {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	child, cancel := context.WithCancel(ctx)
	return &DirectUDPAssociation{
		ctx:     child,
		cancel:  cancel,
		timeout: timeout,
		conns:   make(map[string]net.Conn),
		out:     make(chan directUDPResponse, 64),
	}
}

func (s *DirectUDPAssociation) Send(ctx context.Context, destination string, payload []byte) error {
	if s == nil {
		return fmt.Errorf("direct UDP association is nil")
	}
	conn, err := s.connection(ctx, destination)
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
		defer conn.SetWriteDeadline(time.Time{})
	}
	_, err = conn.Write(payload)
	return err
}

func (s *DirectUDPAssociation) Receive(ctx context.Context) (string, []byte, error) {
	if s == nil {
		return "", nil, fmt.Errorf("direct UDP association is nil")
	}
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-s.ctx.Done():
		return "", nil, s.ctx.Err()
	case result := <-s.out:
		return result.destination, result.payload, result.err
	}
}

func (s *DirectUDPAssociation) Close() error {
	if s == nil {
		return nil
	}
	s.cancel()
	s.mu.Lock()
	conns := s.conns
	s.conns = make(map[string]net.Conn)
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
	return nil
}

func (s *DirectUDPAssociation) connection(ctx context.Context, destination string) (net.Conn, error) {
	s.mu.Lock()
	if conn := s.conns[destination]; conn != nil {
		s.mu.Unlock()
		return conn, nil
	}
	s.mu.Unlock()

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "udp", destination)
	if err != nil {
		return nil, fmt.Errorf("dial direct UDP destination: %w", err)
	}

	s.mu.Lock()
	if existing := s.conns[destination]; existing != nil {
		s.mu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	s.conns[destination] = conn
	s.mu.Unlock()

	go s.readLoop(destination, conn)
	return conn, nil
}

func (s *DirectUDPAssociation) readLoop(destination string, conn net.Conn) {
	buf := make([]byte, 64*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			result := directUDPResponse{
				destination: destination,
				payload:     append([]byte(nil), buf[:n]...),
			}
			select {
			case s.out <- result:
			case <-s.ctx.Done():
				return
			}
		}
		if err != nil {
			s.drop(destination)
			select {
			case s.out <- directUDPResponse{destination: destination, err: err}:
			case <-s.ctx.Done():
			}
			return
		}
	}
}

func (s *DirectUDPAssociation) drop(destination string) {
	s.mu.Lock()
	conn := s.conns[destination]
	delete(s.conns, destination)
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

var _ UDPAssociation = (*DirectUDPAssociation)(nil)
