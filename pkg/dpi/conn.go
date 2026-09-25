package dpi

import (
	"net"
	"sync"
)

// FirstWriteConn applies one userspace DPI strategy only to the first Write.
// Later writes use the original connection directly, so a split workaround
// does not add overhead to the whole video/game/download stream.
type FirstWriteConn struct {
	net.Conn
	Strategy Strategy

	once sync.Once
	err  error
}

// NewFirstWriteConn wraps conn. A nil connection is returned unchanged.
func NewFirstWriteConn(conn net.Conn, strategy Strategy) net.Conn {
	if conn == nil || strategy.Name == "" || strategy.Name == "direct" {
		return conn
	}
	return &FirstWriteConn{Conn: conn, Strategy: strategy}
}

// Write applies the configured strategy once and then becomes a normal conn.
func (c *FirstWriteConn) Write(p []byte) (int, error) {
	applied := false
	c.once.Do(func() {
		applied = true
		c.err = ApplyWriter(c.Conn, p, c.Strategy)
	})
	if applied {
		if c.err != nil {
			return 0, c.err
		}
		return len(p), nil
	}
	return c.Conn.Write(p)
}

// Compile-time check that the wrapper still satisfies net.Conn.
var _ net.Conn = (*FirstWriteConn)(nil)
