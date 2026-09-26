package socks5

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type echoUDPAssociation struct {
	responses chan directUDPResponse
	closeOnce sync.Once
	closed    chan struct{}
}

func newEchoUDPAssociation() *echoUDPAssociation {
	return &echoUDPAssociation{
		responses: make(chan directUDPResponse, 8),
		closed:    make(chan struct{}),
	}
}

func (s *echoUDPAssociation) Send(ctx context.Context, destination string, payload []byte) error {
	response := directUDPResponse{
		destination: destination,
		payload:     append([]byte(nil), payload...),
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return net.ErrClosed
	case s.responses <- response:
		return nil
	}
}

func (s *echoUDPAssociation) Receive(ctx context.Context) (string, []byte, error) {
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-s.closed:
		return "", nil, net.ErrClosed
	case response := <-s.responses:
		return response.destination, response.payload, response.err
	}
}

func (s *echoUDPAssociation) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func TestSOCKS5UDPAssociateRoundTrip(t *testing.T) {
	client, serverConn := net.Pipe()
	defer client.Close()

	assoc := newEchoUDPAssociation()
	server := &Server{
		Dial: func(context.Context, string) (net.Conn, error) {
			return nil, net.ErrClosed
		},
		OpenUDP: func(context.Context) (UDPAssociation, error) {
			return assoc, nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- server.handle(context.Background(), serverConn)
	}()

	if _, err := client.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(client, greeting); err != nil {
		t.Fatal(err)
	}
	if greeting[0] != 0x05 || greeting[1] != 0x00 {
		t.Fatalf("unexpected greeting reply %v", greeting)
	}

	// UDP ASSOCIATE with 0.0.0.0:0 lets the server choose the local relay.
	request := []byte{0x05, 0x03, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 || reply[3] != 0x01 {
		t.Fatalf("unexpected UDP ASSOCIATE reply %v", reply)
	}
	relayPort := int(binary.BigEndian.Uint16(reply[8:10]))
	if relayPort == 0 {
		t.Fatal("SOCKS5 UDP relay returned port zero")
	}

	udpClient, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: relayPort,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer udpClient.Close()

	packet, err := buildUDPDatagram("example.com:53", []byte("dns-query"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := udpClient.Write(packet); err != nil {
		t.Fatal(err)
	}
	if err := udpClient.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	n, err := udpClient.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	destination, payload, err := parseUDPDatagram(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	if destination != "example.com:53" || string(payload) != "dns-query" {
		t.Fatalf("unexpected UDP round trip: %s %q", destination, payload)
	}

	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SOCKS UDP handler close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SOCKS UDP handler did not stop")
	}
}

func TestSOCKSUDPFramingIPv4IPv6AndDomain(t *testing.T) {
	destinations := []string{
		"1.1.1.1:53",
		"[2001:4860:4860::8888]:53",
		"example.com:443",
	}
	for _, destination := range destinations {
		packet, err := buildUDPDatagram(destination, []byte("payload"))
		if err != nil {
			t.Fatalf("%s: %v", destination, err)
		}
		gotDestination, payload, err := parseUDPDatagram(packet)
		if err != nil {
			t.Fatalf("%s: %v", destination, err)
		}
		if gotDestination != destination {
			t.Fatalf("want %q, got %q", destination, gotDestination)
		}
		if string(payload) != "payload" {
			t.Fatalf("unexpected payload %q", payload)
		}
	}
}

func TestSOCKSUDPRejectsFragments(t *testing.T) {
	packet, err := buildUDPDatagram("1.1.1.1:53", []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	packet[2] = 1
	if _, _, err := parseUDPDatagram(packet); err == nil {
		t.Fatal("fragmented SOCKS UDP packet must be rejected")
	}
}

func TestDirectUDPAssociationRoundTrip(t *testing.T) {
	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()

	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := echo.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = echo.WriteToUDP(buf[:n], addr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session := NewDirectUDPAssociation(ctx, time.Second)
	defer session.Close()

	destination := echo.LocalAddr().String()
	if err := session.Send(ctx, destination, []byte("direct-udp")); err != nil {
		t.Fatal(err)
	}
	gotDestination, payload, err := session.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if gotDestination != destination || string(payload) != "direct-udp" {
		t.Fatalf("unexpected direct UDP response: %s %q", gotDestination, payload)
	}
}
