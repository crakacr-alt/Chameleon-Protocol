package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// DNSOverTCPAssociation is a narrow UDP fallback for leak-resistant proxy mode.
// It accepts SOCKS5 UDP DNS requests and carries the DNS payload over a normal
// TCP Chameleon stream (RFC 7766 framing). Non-DNS UDP remains unsupported so
// applications can fall back to their TCP path instead of leaking direct UDP.
type DNSOverTCPAssociation struct {
	dial      DialContextFunc
	responses chan udpResponse
	closed    chan struct{}
	closeOnce sync.Once
}

type udpResponse struct {
	destination string
	payload     []byte
	err         error
}

func NewDNSOverTCPAssociation(dial DialContextFunc) *DNSOverTCPAssociation {
	return &DNSOverTCPAssociation{
		dial:      dial,
		responses: make(chan udpResponse, 16),
		closed:    make(chan struct{}),
	}
}

func (a *DNSOverTCPAssociation) Send(ctx context.Context, destination string, payload []byte) error {
	if a == nil || a.dial == nil {
		return fmt.Errorf("DNS-over-TCP association is not configured")
	}
	host, portText, err := net.SplitHostPort(destination)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port != 53 {
		return fmt.Errorf("UDP fallback only supports DNS port 53")
	}
	_ = host

	conn, err := a.dial(ctx, destination)
	if err != nil {
		return err
	}

	go func() {
		defer conn.Close()
		if len(payload) > 65535 {
			a.push(ctx, udpResponse{err: fmt.Errorf("DNS payload too large")})
			return
		}
		var header [2]byte
		binary.BigEndian.PutUint16(header[:], uint16(len(payload)))
		if err := writeAll(conn, header[:]); err != nil {
			a.push(ctx, udpResponse{err: err})
			return
		}
		if err := writeAll(conn, payload); err != nil {
			a.push(ctx, udpResponse{err: err})
			return
		}
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			a.push(ctx, udpResponse{err: err})
			return
		}
		n := int(binary.BigEndian.Uint16(header[:]))
		if n <= 0 {
			a.push(ctx, udpResponse{err: fmt.Errorf("empty DNS-over-TCP response")})
			return
		}
		reply := make([]byte, n)
		if _, err := io.ReadFull(conn, reply); err != nil {
			a.push(ctx, udpResponse{err: err})
			return
		}
		a.push(ctx, udpResponse{destination: destination, payload: reply})
	}()
	return nil
}

func (a *DNSOverTCPAssociation) Receive(ctx context.Context) (string, []byte, error) {
	if a == nil {
		return "", nil, fmt.Errorf("DNS-over-TCP association is nil")
	}
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-a.closed:
		return "", nil, net.ErrClosed
	case result := <-a.responses:
		return result.destination, result.payload, result.err
	}
}

func (a *DNSOverTCPAssociation) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() { close(a.closed) })
	return nil
}

func (a *DNSOverTCPAssociation) push(ctx context.Context, result udpResponse) {
	select {
	case <-ctx.Done():
	case <-a.closed:
	case a.responses <- result:
	case <-time.After(2 * time.Second):
	}
}

var _ UDPAssociation = (*DNSOverTCPAssociation)(nil)
