package socks5

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestDNSOverTCPAssociation(t *testing.T) {
	dial := func(context.Context, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			var header [2]byte
			if _, err := io.ReadFull(server, header[:]); err != nil {
				return
			}
			n := int(binary.BigEndian.Uint16(header[:]))
			payload := make([]byte, n)
			if _, err := io.ReadFull(server, payload); err != nil {
				return
			}
			binary.BigEndian.PutUint16(header[:], uint16(len(payload)))
			_, _ = server.Write(header[:])
			_, _ = server.Write(payload)
		}()
		return client, nil
	}

	assoc := NewDNSOverTCPAssociation(dial)
	defer assoc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	want := []byte{0x12, 0x34, 0x01, 0x00}
	if err := assoc.Send(ctx, "1.1.1.1:53", want); err != nil {
		t.Fatal(err)
	}
	destination, got, err := assoc.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if destination != "1.1.1.1:53" {
		t.Fatalf("unexpected destination %q", destination)
	}
	if string(got) != string(want) {
		t.Fatalf("unexpected DNS payload %x", got)
	}
}

func TestDNSOverTCPAssociationRejectsNonDNSUDP(t *testing.T) {
	assoc := NewDNSOverTCPAssociation(func(context.Context, string) (net.Conn, error) {
		t.Fatal("dial must not run for non-DNS UDP")
		return nil, nil
	})
	defer assoc.Close()

	if err := assoc.Send(context.Background(), "8.8.8.8:443", []byte("x")); err == nil {
		t.Fatal("expected non-DNS UDP to be rejected")
	}
}
