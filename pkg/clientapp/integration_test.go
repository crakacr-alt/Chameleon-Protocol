package clientapp

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
)

func TestRuntimeSmartSOCKSEndToEnd(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()

	go func() {
		conn, acceptErr := echo.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	cfg := clientconfig.Default()
	cfg.Listen = proxyListener.Addr().String()
	cfg.StateDir = t.TempDir()

	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runtime.Serve(ctx, proxyListener)
	}()

	client, err := net.DialTimeout("tcp", proxyListener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(client, greeting); err != nil {
		t.Fatal(err)
	}
	if greeting[0] != 0x05 || greeting[1] != 0x00 {
		t.Fatalf("unexpected greeting %v", greeting)
	}

	host, portText, err := net.SplitHostPort(echo.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ip := net.ParseIP(host).To4()
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	request := make([]byte, 10)
	request[0] = 0x05
	request[1] = 0x01
	request[2] = 0x00
	request[3] = 0x01
	copy(request[4:8], ip)
	binary.BigEndian.PutUint16(request[8:10], uint16(port))
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(client)
	reply := make([]byte, 10)
	if _, err := io.ReadFull(reader, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 {
		t.Fatalf("SOCKS connect failed with code %d", reply[1])
	}

	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 5)
	if _, err := io.ReadFull(reader, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("unexpected echo %q", got)
	}

	cancel()
	_ = proxyListener.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not stop")
	}
}
