package tunnel

import (
	"context"
	"fmt"
	"net"
	"time"
)

// DialContext opens one authenticated Chameleon TCP tunnel to destination.
func DialContext(ctx context.Context, serverAddress, destination, psk string, timeout time.Duration) (net.Conn, error) {
	if stringsTrim(serverAddress) == "" {
		return nil, fmt.Errorf("server address must not be empty")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", serverAddress)
	if err != nil {
		return nil, fmt.Errorf("dial tunnel server: %w", err)
	}

	secure, err := clientHandshake(conn, destination, psk, timeout)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return secure, nil
}

func clientHandshake(conn net.Conn, destination, psk string, timeout time.Duration) (*SecureConn, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection is nil")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	hello, nonce, err := buildClientHello(psk, destination, time.Now())
	if err != nil {
		return nil, err
	}
	if err := writeFull(conn, hello); err != nil {
		return nil, fmt.Errorf("send tunnel hello: %w", err)
	}

	cipher, err := deriveCipher(psk, nonce)
	if err != nil {
		return nil, err
	}
	secure := newSecureConn(conn, cipher)

	status := make([]byte, 4096)
	n, err := secure.Read(status)
	if err != nil {
		return nil, fmt.Errorf("read tunnel status: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("empty tunnel status")
	}
	if status[0] != 0 {
		message := "remote dial failed"
		if n > 1 {
			message = string(status[1:n])
		}
		return nil, fmt.Errorf("%s", message)
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	return secure, nil
}

func stringsTrim(v string) string {
	start := 0
	end := len(v)
	for start < end && (v[start] == ' ' || v[start] == '\t' || v[start] == '\r' || v[start] == '\n') {
		start++
	}
	for end > start && (v[end-1] == ' ' || v[end-1] == '\t' || v[end-1] == '\r' || v[end-1] == '\n') {
		end--
	}
	return v[start:end]
}
