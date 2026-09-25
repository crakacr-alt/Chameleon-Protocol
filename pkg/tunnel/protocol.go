package tunnel

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
	"golang.org/x/crypto/hkdf"
)

const (
	helloMagic        = "CHT1"
	helloVersion byte = 1
	nonceSize         = 32
	tagSize           = 32
	maxDestinationLen = 1024
)

type clientHello struct {
	Timestamp   int64
	Nonce       [nonceSize]byte
	Destination string
}

func buildClientHello(psk, destination string, now time.Time) ([]byte, [nonceSize]byte, error) {
	var nonce [nonceSize]byte
	if strings.TrimSpace(psk) == "" {
		return nil, nonce, fmt.Errorf("psk must not be empty")
	}
	if err := validateDestination(destination); err != nil {
		return nil, nonce, err
	}
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nonce, fmt.Errorf("generate tunnel nonce: %w", err)
	}

	dest := []byte(destination)
	prefixLen := 4 + 1 + 8 + nonceSize + 2 + len(dest)
	buf := make([]byte, prefixLen+tagSize)
	copy(buf[:4], helloMagic)
	buf[4] = helloVersion
	binary.BigEndian.PutUint64(buf[5:13], uint64(now.Unix()))
	copy(buf[13:13+nonceSize], nonce[:])
	binary.BigEndian.PutUint16(buf[13+nonceSize:15+nonceSize], uint16(len(dest)))
	copy(buf[15+nonceSize:prefixLen], dest)

	tag := helloMAC(psk, buf[:prefixLen])
	copy(buf[prefixLen:], tag)
	return buf, nonce, nil
}

func readClientHello(r io.Reader, psk string, now time.Time, maxSkew time.Duration) (clientHello, error) {
	var hello clientHello
	if strings.TrimSpace(psk) == "" {
		return hello, fmt.Errorf("psk must not be empty")
	}

	fixed := make([]byte, 4+1+8+nonceSize+2)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return hello, fmt.Errorf("read tunnel hello: %w", err)
	}
	if string(fixed[:4]) != helloMagic || fixed[4] != helloVersion {
		return hello, fmt.Errorf("invalid tunnel hello")
	}

	hello.Timestamp = int64(binary.BigEndian.Uint64(fixed[5:13]))
	copy(hello.Nonce[:], fixed[13:13+nonceSize])
	destLen := int(binary.BigEndian.Uint16(fixed[13+nonceSize : 15+nonceSize]))
	if destLen <= 0 || destLen > maxDestinationLen {
		return hello, fmt.Errorf("invalid destination length")
	}

	tail := make([]byte, destLen+tagSize)
	if _, err := io.ReadFull(r, tail); err != nil {
		return hello, fmt.Errorf("read tunnel hello tail: %w", err)
	}
	hello.Destination = string(tail[:destLen])
	if err := validateDestination(hello.Destination); err != nil {
		return hello, err
	}

	signed := make([]byte, 0, len(fixed)+destLen)
	signed = append(signed, fixed...)
	signed = append(signed, tail[:destLen]...)
	want := helloMAC(psk, signed)
	if !hmac.Equal(want, tail[destLen:]) {
		return hello, fmt.Errorf("invalid tunnel authentication")
	}

	if maxSkew <= 0 {
		maxSkew = 2 * time.Minute
	}
	sentAt := time.Unix(hello.Timestamp, 0)
	delta := now.Sub(sentAt)
	if delta < 0 {
		delta = -delta
	}
	if delta > maxSkew {
		return hello, fmt.Errorf("tunnel hello timestamp outside allowed skew")
	}

	return hello, nil
}

func helloMAC(psk string, payload []byte) []byte {
	mac := hmac.New(sha256.New, []byte(psk))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func deriveCipher(psk string, nonce [nonceSize]byte) (*chcrypto.Cipher, error) {
	reader := hkdf.New(sha256.New, []byte(psk), nonce[:], []byte("chameleon/tunnel/v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive tunnel key: %w", err)
	}
	return chcrypto.NewCipherFromKey(key)
}

func validateDestination(destination string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(destination))
	if err != nil {
		return fmt.Errorf("destination must be host:port: %w", err)
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return fmt.Errorf("destination must contain host and port")
	}
	if len(destination) > maxDestinationLen {
		return fmt.Errorf("destination is too long")
	}
	return nil
}
