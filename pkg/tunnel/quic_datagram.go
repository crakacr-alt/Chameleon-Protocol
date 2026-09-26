package tunnel

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
	quic "github.com/quic-go/quic-go"
)

const (
	quicDatagramALPN                = "chameleon-quic-dgram/1"
	datagramSessionDestination      = "udp.session:1"
	datagramFrameVersion       byte = 1
	maxDatagramPlain                = 1080
	maxDatagramDestinations         = 64
)

// ErrDatagramTooLarge is returned before sending a payload that cannot fit in
// Chameleon's conservative QUIC DATAGRAM envelope.
//
// 0.9.1 deliberately does not fragment application UDP packets: preserving
// boundaries and failing explicitly is safer than silently changing semantics.
var ErrDatagramTooLarge = errors.New("UDP datagram is too large for Chameleon QUIC envelope")

// QUICDatagramSession is one authenticated bidirectional UDP association.
//
// QUIC protects packets on the wire with TLS 1.3. Chameleon additionally uses
// the existing PSK-derived AEAD key for destination + payload frames so the
// datagram mode follows the same authentication model as stream tunnels.
type QUICDatagramSession struct {
	conn   *quic.Conn
	cipher *chcrypto.Cipher

	sendMu sync.Mutex
}

// DialQUICDatagramSession creates an authenticated QUIC DATAGRAM association.
func DialQUICDatagramSession(
	ctx context.Context,
	serverAddress string,
	psk string,
	timeout time.Duration,
	tlsCfg TLSClientConfig,
) (*QUICDatagramSession, error) {
	if stringsTrim(serverAddress) == "" {
		return nil, fmt.Errorf("QUIC server address must not be empty")
	}
	if stringsTrim(psk) == "" {
		return nil, fmt.Errorf("psk must not be empty")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	tlsConfig, err := buildTLSClientConfig(serverAddress, tlsCfg)
	if err != nil {
		return nil, err
	}
	tlsConfig = tlsConfig.Clone()
	tlsConfig.MinVersion = tls.VersionTLS13
	tlsConfig.NextProtos = []string{quicDatagramALPN}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := quic.DialAddr(dialCtx, serverAddress, tlsConfig, (&QUICConfig{
		HandshakeTimeout: timeout,
		EnableDatagrams:  true,
	}).quicConfig())
	if err != nil {
		return nil, fmt.Errorf("dial QUIC datagram server: %w", err)
	}
	if !conn.ConnectionState().SupportsDatagrams.Local ||
		!conn.ConnectionState().SupportsDatagrams.Remote {
		_ = conn.CloseWithError(1, "datagrams not negotiated")
		return nil, fmt.Errorf("QUIC peer did not negotiate datagram support")
	}

	stream, err := conn.OpenStreamSync(dialCtx)
	if err != nil {
		_ = conn.CloseWithError(1, "open auth stream failed")
		return nil, fmt.Errorf("open QUIC datagram auth stream: %w", err)
	}

	streamConn := &quicStreamConn{Stream: stream, conn: conn}
	if err := streamConn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = streamConn.Close()
		return nil, err
	}

	hello, nonce, err := buildClientHello(psk, datagramSessionDestination, time.Now())
	if err != nil {
		_ = streamConn.Close()
		return nil, err
	}
	if err := writeFull(streamConn, hello); err != nil {
		_ = streamConn.Close()
		return nil, fmt.Errorf("send datagram auth hello: %w", err)
	}

	cipher, err := deriveCipher(psk, nonce)
	if err != nil {
		_ = streamConn.Close()
		return nil, err
	}
	secure := newSecureConn(streamConn, cipher)
	status := make([]byte, 1024)
	n, err := secure.Read(status)
	if err != nil {
		_ = streamConn.Close()
		return nil, fmt.Errorf("read datagram auth status: %w", err)
	}
	if n == 0 || status[0] != 0 {
		_ = streamConn.Close()
		message := "datagram authentication rejected"
		if n > 1 {
			message = string(status[1:n])
		}
		return nil, fmt.Errorf("%s", message)
	}
	if err := streamConn.SetDeadline(time.Time{}); err != nil {
		_ = streamConn.Close()
		return nil, err
	}

	// The control stream stays open for the lifetime of the QUIC connection.
	return &QUICDatagramSession{conn: conn, cipher: cipher}, nil
}

// Send forwards exactly one application UDP datagram.
func (s *QUICDatagramSession) Send(ctx context.Context, destination string, payload []byte) error {
	if s == nil || s.conn == nil || s.cipher == nil {
		return fmt.Errorf("QUIC datagram session is not initialized")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	plain, err := encodeDatagramFrame(destination, payload)
	if err != nil {
		return err
	}

	s.sendMu.Lock()
	encrypted, err := s.cipher.Seal(plain)
	if err == nil {
		err = s.conn.SendDatagram(encrypted)
	}
	s.sendMu.Unlock()
	if err != nil {
		var tooLarge *quic.DatagramTooLargeError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%w: max encrypted payload %d", ErrDatagramTooLarge, tooLarge.MaxDatagramPayloadSize)
		}
		return fmt.Errorf("send QUIC datagram: %w", err)
	}
	return nil
}

// Receive waits for one response datagram.
func (s *QUICDatagramSession) Receive(ctx context.Context) (string, []byte, error) {
	if s == nil || s.conn == nil || s.cipher == nil {
		return "", nil, fmt.Errorf("QUIC datagram session is not initialized")
	}
	encrypted, err := s.conn.ReceiveDatagram(ctx)
	if err != nil {
		return "", nil, err
	}
	plain, err := s.cipher.Open(encrypted)
	if err != nil {
		return "", nil, fmt.Errorf("decrypt QUIC datagram: %w", err)
	}
	return decodeDatagramFrame(plain)
}

func (s *QUICDatagramSession) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.CloseWithError(0, "")
}

func encodeDatagramFrame(destination string, payload []byte) ([]byte, error) {
	if err := validateDestination(destination); err != nil {
		return nil, err
	}
	dest := []byte(destination)
	if len(dest) > maxDestinationLen {
		return nil, fmt.Errorf("UDP destination is too long")
	}

	plainLen := 1 + 2 + len(dest) + len(payload)
	if plainLen > maxDatagramPlain {
		return nil, fmt.Errorf("%w: payload=%d destination=%d max-plain=%d",
			ErrDatagramTooLarge, len(payload), len(dest), maxDatagramPlain)
	}
	out := make([]byte, plainLen)
	out[0] = datagramFrameVersion
	binary.BigEndian.PutUint16(out[1:3], uint16(len(dest)))
	copy(out[3:3+len(dest)], dest)
	copy(out[3+len(dest):], payload)
	return out, nil
}

func decodeDatagramFrame(frame []byte) (string, []byte, error) {
	if len(frame) < 3 || frame[0] != datagramFrameVersion {
		return "", nil, fmt.Errorf("invalid QUIC datagram frame")
	}
	destLen := int(binary.BigEndian.Uint16(frame[1:3]))
	if destLen <= 0 || destLen > maxDestinationLen || len(frame) < 3+destLen {
		return "", nil, fmt.Errorf("invalid QUIC datagram destination length")
	}
	destination := string(frame[3 : 3+destLen])
	if err := validateDestination(destination); err != nil {
		return "", nil, err
	}
	payload := append([]byte(nil), frame[3+destLen:]...)
	return destination, payload, nil
}

type quicDatagramRelay struct {
	server *Server
	conn   *quic.Conn
	cipher *chcrypto.Cipher
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	upstreams map[string]net.Conn
	sendMu    sync.Mutex
}

func (s *Server) handleQUICDatagramConn(ctx context.Context, conn *quic.Conn) error {
	if s == nil || conn == nil {
		return fmt.Errorf("invalid QUIC datagram connection")
	}

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return fmt.Errorf("accept datagram auth stream: %w", err)
	}
	streamConn := &quicStreamConn{Stream: stream, conn: conn}
	if err := streamConn.SetDeadline(time.Now().Add(s.cfg.HandshakeTimeout)); err != nil {
		return err
	}

	hello, err := readClientHello(streamConn, s.cfg.PSK, time.Now(), s.cfg.MaxClockSkew)
	if err != nil {
		return err
	}
	if hello.Destination != datagramSessionDestination {
		return fmt.Errorf("unexpected datagram session destination")
	}
	if !s.acceptNonce(hello.Nonce, time.Now()) {
		return fmt.Errorf("replayed datagram tunnel hello")
	}
	cipher, err := deriveCipher(s.cfg.PSK, hello.Nonce)
	if err != nil {
		return err
	}
	secure := newSecureConn(streamConn, cipher)
	if _, err := secure.Write([]byte{0}); err != nil {
		return fmt.Errorf("send datagram ready status: %w", err)
	}
	if err := streamConn.SetDeadline(time.Time{}); err != nil {
		return err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	relay := &quicDatagramRelay{
		server:    s,
		conn:      conn,
		cipher:    cipher,
		ctx:       sessionCtx,
		cancel:    cancel,
		upstreams: make(map[string]net.Conn),
	}
	defer relay.close()

	for {
		encrypted, err := conn.ReceiveDatagram(sessionCtx)
		if err != nil {
			return err
		}
		plain, err := cipher.Open(encrypted)
		if err != nil {
			continue
		}
		destination, payload, err := decodeDatagramFrame(plain)
		if err != nil {
			continue
		}
		upstream, err := relay.upstream(destination)
		if err != nil {
			continue
		}
		if _, err := upstream.Write(payload); err != nil {
			relay.drop(destination)
		}
	}
}

func (r *quicDatagramRelay) upstream(destination string) (net.Conn, error) {
	r.mu.Lock()
	if conn := r.upstreams[destination]; conn != nil {
		r.mu.Unlock()
		return conn, nil
	}
	if len(r.upstreams) >= maxDatagramDestinations {
		r.mu.Unlock()
		return nil, fmt.Errorf("too many UDP destinations in one association")
	}
	r.mu.Unlock()

	if !r.server.cfg.AllowPrivateDestinations {
		if err := rejectPrivateDestinationLiteral(destination); err != nil {
			return nil, err
		}
	}

	dialCtx, cancel := context.WithTimeout(r.ctx, r.server.cfg.DialTimeout)
	defer cancel()
	upstream, err := r.server.cfg.DialContext(dialCtx, "udp", destination)
	if err != nil {
		return nil, fmt.Errorf("dial UDP destination: %w", err)
	}
	if !r.server.cfg.AllowPrivateDestinations {
		if err := rejectPrivateRemote(upstream.RemoteAddr()); err != nil {
			_ = upstream.Close()
			return nil, err
		}
	}

	r.mu.Lock()
	if existing := r.upstreams[destination]; existing != nil {
		r.mu.Unlock()
		_ = upstream.Close()
		return existing, nil
	}
	if len(r.upstreams) >= maxDatagramDestinations {
		r.mu.Unlock()
		_ = upstream.Close()
		return nil, fmt.Errorf("too many UDP destinations in one association")
	}
	r.upstreams[destination] = upstream
	r.mu.Unlock()

	go r.readResponses(destination, upstream)
	return upstream, nil
}

func (r *quicDatagramRelay) readResponses(destination string, upstream net.Conn) {
	buf := make([]byte, 64*1024)
	for {
		n, err := upstream.Read(buf)
		if n > 0 {
			plain, encodeErr := encodeDatagramFrame(destination, buf[:n])
			if encodeErr == nil {
				r.sendMu.Lock()
				encrypted, sealErr := r.cipher.Seal(plain)
				if sealErr == nil {
					sealErr = r.conn.SendDatagram(encrypted)
				}
				r.sendMu.Unlock()
				if sealErr != nil {
					return
				}
			}
		}
		if err != nil {
			r.drop(destination)
			return
		}
	}
}

func (r *quicDatagramRelay) drop(destination string) {
	r.mu.Lock()
	conn := r.upstreams[destination]
	delete(r.upstreams, destination)
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (r *quicDatagramRelay) close() {
	r.cancel()
	r.mu.Lock()
	upstreams := r.upstreams
	r.upstreams = make(map[string]net.Conn)
	r.mu.Unlock()
	for _, conn := range upstreams {
		_ = conn.Close()
	}
}
