package tunnel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
	"golang.org/x/crypto/hkdf"
)

const (
	helloVersion      byte = 1
	nonceSize              = 32
	maxDestinationLen      = 1024
	maxHelloCipher         = 2048
)

type clientHello struct {
	Timestamp   int64
	Nonce       [nonceSize]byte
	Destination string
}

// buildClientHello keeps the destination and timestamp encrypted.
// The only fixed-size clear field is a random per-connection salt used for KDF.
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
	plain := make([]byte, 1+8+2+len(dest))
	plain[0] = helloVersion
	binary.BigEndian.PutUint64(plain[1:9], uint64(now.Unix()))
	binary.BigEndian.PutUint16(plain[9:11], uint16(len(dest)))
	copy(plain[11:], dest)

	cipher, err := deriveCipher(psk, nonce)
	if err != nil {
		return nil, nonce, err
	}
	encrypted, err := cipher.Seal(plain)
	if err != nil {
		return nil, nonce, fmt.Errorf("encrypt tunnel hello: %w", err)
	}
	if len(encrypted) > maxHelloCipher {
		return nil, nonce, fmt.Errorf("encrypted tunnel hello too large")
	}

	buf := make([]byte, nonceSize+2+len(encrypted))
	copy(buf[:nonceSize], nonce[:])
	binary.BigEndian.PutUint16(buf[nonceSize:nonceSize+2], uint16(len(encrypted)))
	copy(buf[nonceSize+2:], encrypted)
	return buf, nonce, nil
}

func readClientHello(r io.Reader, psk string, now time.Time, maxSkew time.Duration) (clientHello, error) {
	var hello clientHello
	if strings.TrimSpace(psk) == "" {
		return hello, fmt.Errorf("psk must not be empty")
	}

	fixed := make([]byte, nonceSize+2)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return hello, fmt.Errorf("read tunnel hello: %w", err)
	}
	copy(hello.Nonce[:], fixed[:nonceSize])
	encryptedLen := int(binary.BigEndian.Uint16(fixed[nonceSize : nonceSize+2]))
	if encryptedLen <= 0 || encryptedLen > maxHelloCipher {
		return hello, fmt.Errorf("invalid encrypted hello length")
	}

	encrypted := make([]byte, encryptedLen)
	if _, err := io.ReadFull(r, encrypted); err != nil {
		return hello, fmt.Errorf("read encrypted tunnel hello: %w", err)
	}
	cipher, err := deriveCipher(psk, hello.Nonce)
	if err != nil {
		return hello, err
	}
	plain, err := cipher.Open(encrypted)
	if err != nil {
		return hello, fmt.Errorf("invalid tunnel authentication")
	}
	if len(plain) < 11 || plain[0] != helloVersion {
		return hello, fmt.Errorf("invalid tunnel hello")
	}

	hello.Timestamp = int64(binary.BigEndian.Uint64(plain[1:9]))
	destLen := int(binary.BigEndian.Uint16(plain[9:11]))
	if destLen <= 0 || destLen > maxDestinationLen || len(plain) != 11+destLen {
		return hello, fmt.Errorf("invalid destination length")
	}
	hello.Destination = string(plain[11:])
	if err := validateDestination(hello.Destination); err != nil {
		return hello, err
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

func deriveCipher(psk string, nonce [nonceSize]byte) (*chcrypto.Cipher, error) {
	reader := hkdf.New(sha256.New, []byte(psk), nonce[:], []byte("chameleon/tunnel/v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive tunnel key: %w", err)
	}
	return chcrypto.NewCipherFromKey(key)
}

func validateDestination(destination string) error {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(destination))
	if err != nil {
		return fmt.Errorf("destination must be host:port: %w", err)
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(portText) == "" {
		return fmt.Errorf("destination must contain host and port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("destination port is invalid")
	}
	if len(destination) > maxDestinationLen {
		return fmt.Errorf("destination is too long")
	}
	return nil
}
