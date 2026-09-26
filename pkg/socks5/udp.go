package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// UDPAssociation is the transport-neutral side of SOCKS5 UDP ASSOCIATE.
//
// A Chameleon QUIC datagram session implements this interface, but socks5 does
// not depend on the tunnel package. This keeps the local proxy reusable.
type UDPAssociation interface {
	Send(context.Context, string, []byte) error
	Receive(context.Context) (string, []byte, error)
	Close() error
}

// OpenUDPFunc creates one UDP association for one SOCKS5 control connection.
type OpenUDPFunc func(context.Context) (UDPAssociation, error)

func (s *Server) handleUDPAssociate(ctx context.Context, control net.Conn) error {
	if s.OpenUDP == nil {
		_ = writeReply(control, 0x07, nil)
		return fmt.Errorf("SOCKS5 UDP ASSOCIATE is not configured")
	}

	association, err := s.OpenUDP(ctx)
	if err != nil {
		_ = writeReply(control, 0x01, nil)
		return err
	}
	defer association.Close()

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		_ = writeReply(control, 0x01, nil)
		return fmt.Errorf("listen SOCKS UDP relay: %w", err)
	}
	defer udpConn.Close()

	if err := writeReply(control, 0x00, udpConn.LocalAddr()); err != nil {
		return err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		clientMu   sync.RWMutex
		clientAddr *net.UDPAddr
		bytesUp    atomic.Int64
		bytesDown  atomic.Int64
	)

	setClient := func(addr *net.UDPAddr) bool {
		clientMu.Lock()
		defer clientMu.Unlock()
		if clientAddr == nil {
			clientAddr = cloneUDPAddr(addr)
			return true
		}
		return clientAddr.IP.Equal(addr.IP) && clientAddr.Port == addr.Port
	}
	getClient := func() *net.UDPAddr {
		clientMu.RLock()
		defer clientMu.RUnlock()
		return cloneUDPAddr(clientAddr)
	}

	errCh := make(chan error, 3)

	// RFC 1928 keeps UDP ASSOCIATE alive for as long as its TCP control
	// connection exists. Reading to EOF gives us a clean lifetime signal.
	go func() {
		buf := make([]byte, 1)
		for {
			if _, err := control.Read(buf); err != nil {
				errCh <- err
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, addr, err := udpConn.ReadFromUDP(buf)
			if err != nil {
				errCh <- err
				return
			}
			if !setClient(addr) {
				continue
			}
			destination, payload, err := parseUDPDatagram(buf[:n])
			if err != nil {
				continue
			}
			if err := association.Send(sessionCtx, destination, payload); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
					errCh <- err
					return
				}
				// An oversized or otherwise invalid application datagram should
				// not tear down the whole association.
				continue
			}
			bytesUp.Add(int64(len(payload)))
		}
	}()

	go func() {
		for {
			destination, payload, err := association.Receive(sessionCtx)
			if err != nil {
				errCh <- err
				return
			}
			addr := getClient()
			if addr == nil {
				continue
			}
			packet, err := buildUDPDatagram(destination, payload)
			if err != nil {
				continue
			}
			if _, err := udpConn.WriteToUDP(packet, addr); err != nil {
				errCh <- err
				return
			}
			bytesDown.Add(int64(len(payload)))
		}
	}()

	started := time.Now()
	firstErr := <-errCh
	cancel()
	_ = udpConn.Close()
	_ = association.Close()

	sessionErr := firstErr
	if errors.Is(firstErr, net.ErrClosed) ||
		errors.Is(firstErr, context.Canceled) ||
		errors.Is(firstErr, context.DeadlineExceeded) {
		sessionErr = nil
	}

	s.emit(SessionResult{
		Destination: "UDP ASSOCIATE",
		Duration:    time.Since(started),
		BytesUp:     bytesUp.Load(),
		BytesDown:   bytesDown.Load(),
		Err:         sessionErr,
	})
	return sessionErr
}

// parseUDPDatagram parses the RFC 1928 UDP request header.
// Fragmented SOCKS datagrams are rejected; QUIC already handles transport
// packetization, and silently reassembling SOCKS FRAG would hide semantics.
func parseUDPDatagram(packet []byte) (string, []byte, error) {
	if len(packet) < 4 {
		return "", nil, fmt.Errorf("SOCKS UDP packet too short")
	}
	if packet[0] != 0 || packet[1] != 0 {
		return "", nil, fmt.Errorf("invalid SOCKS UDP reserved bytes")
	}
	if packet[2] != 0 {
		return "", nil, fmt.Errorf("fragmented SOCKS UDP packet is not supported")
	}

	offset := 3
	host, used, err := parseAddressBytes(packet[offset:])
	if err != nil {
		return "", nil, err
	}
	offset += used
	if len(packet) < offset+2 {
		return "", nil, fmt.Errorf("SOCKS UDP packet missing port")
	}
	port := binary.BigEndian.Uint16(packet[offset : offset+2])
	offset += 2
	if port == 0 {
		return "", nil, fmt.Errorf("SOCKS UDP destination port is zero")
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port))), append([]byte(nil), packet[offset:]...), nil
}

func buildUDPDatagram(destination string, payload []byte) ([]byte, error) {
	host, portText, err := net.SplitHostPort(destination)
	if err != nil {
		return nil, fmt.Errorf("invalid SOCKS UDP destination: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid SOCKS UDP destination port")
	}

	address, err := encodeAddress(host)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 3+len(address)+2+len(payload))
	// RSV=0, FRAG=0
	copy(out[3:], address)
	offset := 3 + len(address)
	binary.BigEndian.PutUint16(out[offset:offset+2], uint16(port))
	copy(out[offset+2:], payload)
	return out, nil
}

func parseAddressBytes(packet []byte) (string, int, error) {
	if len(packet) < 1 {
		return "", 0, fmt.Errorf("missing SOCKS UDP address type")
	}
	switch packet[0] {
	case 0x01:
		if len(packet) < 5 {
			return "", 0, fmt.Errorf("short IPv4 SOCKS UDP address")
		}
		return net.IP(packet[1:5]).String(), 5, nil
	case 0x03:
		if len(packet) < 2 {
			return "", 0, fmt.Errorf("short domain SOCKS UDP address")
		}
		length := int(packet[1])
		if length == 0 || len(packet) < 2+length {
			return "", 0, fmt.Errorf("invalid domain SOCKS UDP address")
		}
		return string(packet[2 : 2+length]), 2 + length, nil
	case 0x04:
		if len(packet) < 17 {
			return "", 0, fmt.Errorf("short IPv6 SOCKS UDP address")
		}
		return net.IP(packet[1:17]).String(), 17, nil
	default:
		return "", 0, fmt.Errorf("unsupported SOCKS UDP address type %d", packet[0])
	}
}

func encodeAddress(host string) ([]byte, error) {
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return append([]byte{0x01}, ip4...), nil
		}
		ip16 := ip.To16()
		if ip16 == nil {
			return nil, fmt.Errorf("invalid IP address")
		}
		return append([]byte{0x04}, ip16...), nil
	}
	if len(host) == 0 || len(host) > 255 {
		return nil, fmt.Errorf("invalid SOCKS domain length")
	}
	out := make([]byte, 2+len(host))
	out[0] = 0x03
	out[1] = byte(len(host))
	copy(out[2:], host)
	return out, nil
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	copyAddr := *addr
	copyAddr.IP = append(net.IP(nil), addr.IP...)
	return &copyAddr
}
