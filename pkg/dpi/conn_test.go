package dpi

import (
	"bytes"
	"net"
	"testing"
	"time"
)

type bufferConn struct {
	bytes.Buffer
}

func (c *bufferConn) Read([]byte) (int, error)         { return 0, nil }
func (c *bufferConn) Close() error                     { return nil }
func (c *bufferConn) LocalAddr() net.Addr              { return testAddr("local") }
func (c *bufferConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (c *bufferConn) SetDeadline(time.Time) error      { return nil }
func (c *bufferConn) SetReadDeadline(time.Time) error  { return nil }
func (c *bufferConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

func TestFirstWriteConnPreservesData(t *testing.T) {
	base := &bufferConn{}
	strategy := Strategy{
		Name:        "split",
		Techniques:  []Technique{TechniqueSplit},
		SplitPoints: []int{1, 3},
	}
	conn := NewFirstWriteConn(base, strategy)

	if _, err := conn.Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("XYZ")); err != nil {
		t.Fatal(err)
	}
	if got := base.String(); got != "abcdefXYZ" {
		t.Fatalf("unexpected data %q", got)
	}
}
