package socks5

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestSOCKS5ServerAndClientHandshake(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	upstreamServer, upstreamEcho := net.Pipe()

	go func() {
		_, _ = io.Copy(upstreamEcho, upstreamEcho)
		_ = upstreamEcho.Close()
	}()

	server := &Server{
		Dial: func(context.Context, string) (net.Conn, error) {
			return upstreamServer, nil
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- server.handle(context.Background(), serverSide)
	}()

	if err := clientHandshake(clientSide, "example.com:443"); err != nil {
		t.Fatal(err)
	}
	if _, err := clientSide.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 5)
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "hello" {
		t.Fatalf("unexpected reply %q", reply)
	}

	_ = clientSide.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SOCKS5 handler did not stop")
	}
}

func TestReadRequestDomain(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		request := []byte{
			0x05, 0x01, 0x00, 0x03,
			byte(len("example.com")),
		}
		request = append(request, []byte("example.com")...)
		request = append(request, 0x01, 0xbb)
		_, _ = client.Write(request)
	}()

	got, err := readRequest(server)
	if err != nil {
		t.Fatal(err)
	}
	if got != "example.com:443" {
		t.Fatalf("unexpected destination %q", got)
	}
}
