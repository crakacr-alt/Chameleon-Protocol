package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// RawDialFunc opens the TCP connection to the SOCKS5 relay.
type RawDialFunc func(context.Context, string) (net.Conn, error)

// DialContext connects to destination through an existing SOCKS5 proxy.
// It supports the no-auth method, which is suitable for a localhost sidecar.
func DialContext(ctx context.Context, proxyAddress, destination string, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	return DialContextWithDialer(ctx, proxyAddress, destination, timeout, func(ctx context.Context, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", address)
	})
}

// DialContextWithDialer lets callers wrap the first hop before the SOCKS
// greeting is sent. Chameleon uses this to apply a learned userspace strategy
// to an existing relay connection without owning another VPN interface.
func DialContextWithDialer(ctx context.Context, proxyAddress, destination string, timeout time.Duration, dial RawDialFunc) (net.Conn, error) {
	if strings.TrimSpace(proxyAddress) == "" {
		return nil, fmt.Errorf("proxy address must not be empty")
	}
	if dial == nil {
		return nil, fmt.Errorf("raw dial function is nil")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	conn, err := dial(ctx, proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("dial SOCKS5 proxy: %w", err)
	}

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := clientHandshake(conn, destination); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func clientHandshake(conn net.Conn, destination string) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	var greeting [2]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return err
	}
	if greeting != [2]byte{0x05, 0x00} {
		return fmt.Errorf("SOCKS5 proxy rejected no-auth method")
	}

	host, portText, err := net.SplitHostPort(destination)
	if err != nil {
		return fmt.Errorf("destination must be host:port: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid destination port")
	}

	request := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			request = append(request, 0x01)
			request = append(request, ip4...)
		} else {
			request = append(request, 0x04)
			request = append(request, ip.To16()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return fmt.Errorf("invalid destination host")
		}
		request = append(request, 0x03, byte(len(host)))
		request = append(request, host...)
	}
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	request = append(request, portBuf[:]...)

	if _, err := conn.Write(request); err != nil {
		return err
	}

	var reply [4]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return err
	}
	if reply[0] != 0x05 {
		return fmt.Errorf("invalid SOCKS5 reply")
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("SOCKS5 CONNECT failed with code %d", reply[1])
	}
	if _, err := readAddress(conn, reply[3]); err != nil {
		return err
	}
	var boundPort [2]byte
	if _, err := io.ReadFull(conn, boundPort[:]); err != nil {
		return err
	}
	return nil
}
