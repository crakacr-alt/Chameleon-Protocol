package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// DialContextFunc opens the selected outbound path for a SOCKS destination.
type DialContextFunc func(context.Context, string) (net.Conn, error)

// SessionResult is emitted after a proxied connection finishes.
type SessionResult struct {
	Destination string
	Duration    time.Duration
	BytesUp     int64
	BytesDown   int64
	Err         error
}

// Server is a small SOCKS5 CONNECT server.
// Authentication is intentionally omitted because the default deployment
// binds to localhost; callers exposing it remotely must add an outer access layer.
type Server struct {
	Dial      DialContextFunc
	OpenUDP   OpenUDPFunc
	OnSession func(SessionResult)
}

// Serve accepts SOCKS clients until listener.Close or context cancellation.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if s == nil || (s.Dial == nil && s.OpenUDP == nil) {
		return fmt.Errorf("SOCKS5 requires at least one outbound transport")
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
			_ = s.handle(ctx, conn)
		}()
	}
}

func (s *Server) handle(ctx context.Context, client net.Conn) error {
	defer client.Close()

	if err := serverGreeting(client); err != nil {
		return err
	}
	command, destination, err := readRequestDetails(client)
	if err != nil {
		_ = writeReply(client, 0x01, nil)
		return err
	}

	if command == 0x03 {
		return s.handleUDPAssociate(ctx, client)
	}
	if command != 0x01 {
		_ = writeReply(client, 0x07, nil)
		return fmt.Errorf("unsupported SOCKS5 command %d", command)
	}
	if s.Dial == nil {
		_ = writeReply(client, 0x07, nil)
		return fmt.Errorf("SOCKS5 CONNECT is not configured")
	}

	started := time.Now()
	upstream, err := s.Dial(ctx, destination)
	if err != nil {
		_ = writeReply(client, 0x05, nil)
		s.emit(SessionResult{Destination: destination, Duration: time.Since(started), Err: err})
		return err
	}
	defer upstream.Close()

	if err := writeReply(client, 0x00, upstream.LocalAddr()); err != nil {
		return err
	}

	type copyResult struct {
		n   int64
		err error
		up  bool
	}
	results := make(chan copyResult, 2)
	go func() {
		n, copyErr := io.Copy(upstream, client)
		results <- copyResult{n: n, err: copyErr, up: true}
	}()
	go func() {
		n, copyErr := io.Copy(client, upstream)
		results <- copyResult{n: n, err: copyErr, up: false}
	}()

	first := <-results
	_ = upstream.Close()
	_ = client.Close()
	second := <-results

	var bytesUp, bytesDown int64
	var sessionErr error
	for _, result := range []copyResult{first, second} {
		if result.up {
			bytesUp = result.n
		} else {
			bytesDown = result.n
		}
		if sessionErr == nil && result.err != nil && !isClosed(result.err) {
			sessionErr = result.err
		}
	}

	s.emit(SessionResult{
		Destination: destination,
		Duration:    time.Since(started),
		BytesUp:     bytesUp,
		BytesDown:   bytesDown,
		Err:         sessionErr,
	})
	return sessionErr
}

func (s *Server) emit(result SessionResult) {
	if s.OnSession != nil {
		s.OnSession(result)
	}
}

func serverGreeting(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != 0x05 {
		return fmt.Errorf("unsupported SOCKS version %d", header[0])
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	foundNoAuth := false
	for _, method := range methods {
		if method == 0x00 {
			foundNoAuth = true
			break
		}
	}
	if !foundNoAuth {
		_ = writeAll(conn, []byte{0x05, 0xff})
		return fmt.Errorf("client did not offer no-auth method")
	}
	return writeAll(conn, []byte{0x05, 0x00})
}

func readRequest(conn net.Conn) (string, error) {
	command, destination, err := readRequestDetails(conn)
	if err != nil {
		return "", err
	}
	if command != 0x01 {
		return "", fmt.Errorf("SOCKS5 command is not CONNECT")
	}
	return destination, nil
}

func readRequestDetails(conn net.Conn) (byte, string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, "", err
	}
	if header[0] != 0x05 || header[2] != 0x00 {
		return 0, "", fmt.Errorf("invalid SOCKS5 request header")
	}

	host, err := readAddress(conn, header[3])
	if err != nil {
		return 0, "", err
	}
	var portBytes [2]byte
	if _, err := io.ReadFull(conn, portBytes[:]); err != nil {
		return 0, "", err
	}
	port := binary.BigEndian.Uint16(portBytes[:])
	return header[1], net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

func readAddress(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case 0x01:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case 0x03:
		var length [1]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return "", err
		}
		if length[0] == 0 {
			return "", fmt.Errorf("empty domain")
		}
		buf := make([]byte, int(length[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	case 0x04:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	default:
		return "", fmt.Errorf("unsupported address type %d", atyp)
	}
}

func writeReply(conn net.Conn, code byte, addr net.Addr) error {
	ip := net.IPv4zero
	port := 0

	switch value := addr.(type) {
	case *net.TCPAddr:
		ip = value.IP
		port = value.Port
	case *net.UDPAddr:
		ip = value.IP
		port = value.Port
	}
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero
	}

	reply := make([]byte, 10)
	reply[0] = 0x05
	reply[1] = code
	reply[2] = 0x00
	reply[3] = 0x01
	copy(reply[4:8], ip4)
	binary.BigEndian.PutUint16(reply[8:10], uint16(port))
	return writeAll(conn, reply)
}

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(p) {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

func isClosed(err error) bool {
	return err == nil || errors.Is(err, net.ErrClosed)
}
