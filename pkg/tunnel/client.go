package tunnel

import (
	"context"
	"fmt"
	"net"
	"time"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
)

// RawDialFunc opens the TCP connection to the Chameleon server.
type RawDialFunc func(context.Context, string) (net.Conn, error)

// DialContext opens one legacy-PSK authenticated Chameleon TCP tunnel.
func DialContext(ctx context.Context, serverAddress, destination, psk string, timeout time.Duration) (net.Conn, error) {
	return DialContextAuth(ctx, serverAddress, destination, ClientAuth{PSK: psk}, timeout)
}

// DialContextAuth opens one Chameleon TCP tunnel using Auth v2 when
// ClientID/ClientSecret are present, otherwise the legacy PSK handshake.
func DialContextAuth(ctx context.Context, serverAddress, destination string, auth ClientAuth, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	return DialContextWithDialerAuth(ctx, serverAddress, destination, auth, timeout, func(ctx context.Context, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", address)
	})
}

// DialContextWithDialer is the same tunnel dial but lets the caller wrap the
// first hop. The local proxy uses this to apply a learned first-write strategy
// only to the connection visible to the local network.
func DialContextWithDialer(ctx context.Context, serverAddress, destination, psk string, timeout time.Duration, dial RawDialFunc) (net.Conn, error) {
	return DialContextWithDialerAuth(ctx, serverAddress, destination, ClientAuth{PSK: psk}, timeout, dial)
}

func DialContextWithDialerAuth(ctx context.Context, serverAddress, destination string, auth ClientAuth, timeout time.Duration, dial RawDialFunc) (net.Conn, error) {
	if stringsTrim(serverAddress) == "" {
		return nil, fmt.Errorf("server address must not be empty")
	}
	if dial == nil {
		return nil, fmt.Errorf("raw dial function is nil")
	}
	if err := auth.Validate(); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	conn, err := dial(ctx, serverAddress)
	if err != nil {
		return nil, fmt.Errorf("dial tunnel server: %w", err)
	}

	secure, err := clientHandshakeAuth(conn, destination, auth, timeout)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return secure, nil
}

func clientHandshake(conn net.Conn, destination, psk string, timeout time.Duration) (*SecureConn, error) {
	return clientHandshakeAuth(conn, destination, ClientAuth{PSK: psk}, timeout)
}

func clientHandshakeAuth(conn net.Conn, destination string, auth ClientAuth, timeout time.Duration) (*SecureConn, error) {
	secure, _, err := clientHandshakeAuthWithCipher(conn, destination, auth, timeout)
	return secure, err
}

func clientHandshakeAuthWithCipher(conn net.Conn, destination string, auth ClientAuth, timeout time.Duration) (*SecureConn, *chcrypto.Cipher, error) {
	if auth.UsesV2() {
		return clientHandshakeV2WithCipher(conn, destination, auth, timeout)
	}
	return clientHandshakeLegacyWithCipher(conn, destination, auth.PSK, timeout)
}

func clientHandshakeLegacy(conn net.Conn, destination, psk string, timeout time.Duration) (*SecureConn, error) {
	secure, _, err := clientHandshakeLegacyWithCipher(conn, destination, psk, timeout)
	return secure, err
}

func clientHandshakeLegacyWithCipher(conn net.Conn, destination, psk string, timeout time.Duration) (*SecureConn, *chcrypto.Cipher, error) {
	if conn == nil {
		return nil, nil, fmt.Errorf("connection is nil")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, nil, err
	}

	hello, nonce, err := buildClientHello(psk, destination, time.Now())
	if err != nil {
		return nil, nil, err
	}
	if err := writeFull(conn, hello); err != nil {
		return nil, nil, fmt.Errorf("send tunnel hello: %w", err)
	}

	cipher, err := deriveCipher(psk, nonce)
	if err != nil {
		return nil, nil, err
	}
	secure := newSecureConn(conn, cipher)

	status := make([]byte, 4096)
	n, err := secure.Read(status)
	if err != nil {
		return nil, nil, fmt.Errorf("read tunnel status: %w", err)
	}
	if n == 0 {
		return nil, nil, fmt.Errorf("empty tunnel status")
	}
	if status[0] != 0 {
		message := "remote dial failed"
		if n > 1 {
			message = string(status[1:n])
		}
		return nil, nil, fmt.Errorf("%s", message)
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, nil, err
	}
	return secure, cipher, nil
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
