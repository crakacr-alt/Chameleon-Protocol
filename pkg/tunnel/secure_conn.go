package tunnel

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
)

const (
	maxPlainFrame  = 64 * 1024
	maxCipherFrame = 128 * 1024
)

// SecureConn turns a byte stream into independently authenticated AEAD frames.
// It is intentionally small: the tunnel keeps TCP ordering, while AEAD prevents
// the proxy destination and payload from being accepted after modification.
type SecureConn struct {
	conn   net.Conn
	cipher *chcrypto.Cipher

	readMu  sync.Mutex
	writeMu sync.Mutex
	pending []byte
}

func newSecureConn(conn net.Conn, cipher *chcrypto.Cipher) *SecureConn {
	return &SecureConn{conn: conn, cipher: cipher}
}

func (c *SecureConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	c.readMu.Lock()
	defer c.readMu.Unlock()

	if len(c.pending) == 0 {
		var header [4]byte
		if _, err := io.ReadFull(c.conn, header[:]); err != nil {
			return 0, err
		}
		length := int(binary.BigEndian.Uint32(header[:]))
		if length <= 0 || length > maxCipherFrame {
			return 0, fmt.Errorf("invalid encrypted frame length %d", length)
		}
		encrypted := make([]byte, length)
		if _, err := io.ReadFull(c.conn, encrypted); err != nil {
			return 0, err
		}
		plain, err := c.cipher.Open(encrypted)
		if err != nil {
			return 0, fmt.Errorf("decrypt tunnel frame: %w", err)
		}
		c.pending = plain
	}

	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func (c *SecureConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	written := 0
	for len(p) > 0 {
		size := len(p)
		if size > maxPlainFrame {
			size = maxPlainFrame
		}
		chunk := p[:size]
		encrypted, err := c.cipher.Seal(chunk)
		if err != nil {
			return written, fmt.Errorf("encrypt tunnel frame: %w", err)
		}
		if len(encrypted) > maxCipherFrame {
			return written, fmt.Errorf("encrypted frame too large")
		}

		var header [4]byte
		binary.BigEndian.PutUint32(header[:], uint32(len(encrypted)))
		if err := writeFull(c.conn, header[:]); err != nil {
			return written, err
		}
		if err := writeFull(c.conn, encrypted); err != nil {
			return written, err
		}
		written += size
		p = p[size:]
	}
	return written, nil
}

func (c *SecureConn) Close() error                       { return c.conn.Close() }
func (c *SecureConn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *SecureConn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *SecureConn) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *SecureConn) SetReadDeadline(t time.Time) error  { return c.conn.SetReadDeadline(t) }
func (c *SecureConn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }

func writeFull(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(p) {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

var _ net.Conn = (*SecureConn)(nil)
