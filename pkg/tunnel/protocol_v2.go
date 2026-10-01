package tunnel

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
	"golang.org/x/crypto/hkdf"
)

var authV2Magic = []byte{0x43, 0x48, 0x32, 0x00} // CH2\0

const (
	authV2Version byte = 2
	maxClientIDLen     = 128
)

// clientHandshakeV2 authenticates a per-client identity without relying on
// wall-clock timestamps. A fresh server challenge makes recorded proofs
// unusable on a later connection.
func clientHandshakeV2(conn net.Conn, destination string, auth ClientAuth, timeout time.Duration) (*SecureConn, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection is nil")
	}
	if err := auth.Validate(); err != nil {
		return nil, err
	}
	if !auth.UsesV2() {
		return nil, fmt.Errorf("Auth v2 credentials are required")
	}
	if err := validateDestination(destination); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	var clientNonce [nonceSize]byte
	if _, err := rand.Read(clientNonce[:]); err != nil {
		return nil, fmt.Errorf("generate client nonce: %w", err)
	}
	id := []byte(strings.TrimSpace(auth.ClientID))
	start := make([]byte, len(authV2Magic)+2+len(id)+nonceSize)
	copy(start, authV2Magic)
	binary.BigEndian.PutUint16(start[len(authV2Magic):len(authV2Magic)+2], uint16(len(id)))
	copy(start[len(authV2Magic)+2:], id)
	copy(start[len(authV2Magic)+2+len(id):], clientNonce[:])
	if err := writeFull(conn, start); err != nil {
		return nil, fmt.Errorf("send Auth v2 hello: %w", err)
	}

	var serverNonce [nonceSize]byte
	if _, err := io.ReadFull(conn, serverNonce[:]); err != nil {
		return nil, fmt.Errorf("read Auth v2 challenge: %w", err)
	}

	authCipher, err := deriveV2Cipher(auth.ClientSecret, clientNonce, serverNonce, "chameleon/auth/v2")
	if err != nil {
		return nil, err
	}
	dest := []byte(destination)
	plain := make([]byte, 1+2+len(dest))
	plain[0] = authV2Version
	binary.BigEndian.PutUint16(plain[1:3], uint16(len(dest)))
	copy(plain[3:], dest)
	encrypted, err := authCipher.Seal(plain)
	if err != nil {
		return nil, fmt.Errorf("encrypt Auth v2 proof: %w", err)
	}
	if len(encrypted) > maxHelloCipher {
		return nil, fmt.Errorf("Auth v2 proof is too large")
	}
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(encrypted)))
	if err := writeFull(conn, length[:]); err != nil {
		return nil, fmt.Errorf("send Auth v2 proof length: %w", err)
	}
	if err := writeFull(conn, encrypted); err != nil {
		return nil, fmt.Errorf("send Auth v2 proof: %w", err)
	}

	sessionCipher, err := deriveV2Cipher(auth.ClientSecret, clientNonce, serverNonce, "chameleon/tunnel/v2/session")
	if err != nil {
		return nil, err
	}
	secure := newSecureConn(conn, sessionCipher)
	status := make([]byte, 4096)
	n, err := secure.Read(status)
	if err != nil {
		return nil, fmt.Errorf("read tunnel status: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("empty tunnel status")
	}
	if status[0] != 0 {
		message := "remote dial failed"
		if n > 1 {
			message = string(status[1:n])
		}
		return nil, fmt.Errorf("%s", message)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	return secure, nil
}

func serverHandshakeV2(r io.Reader, w io.Writer, registry *ClientRegistry, now time.Time) (clientHello, *chcrypto.Cipher, error) {
	var hello clientHello
	magic := make([]byte, len(authV2Magic))
	if _, err := io.ReadFull(r, magic); err != nil {
		return hello, nil, fmt.Errorf("read Auth v2 magic: %w", err)
	}
	if !bytes.Equal(magic, authV2Magic) {
		return hello, nil, fmt.Errorf("invalid Auth v2 preface")
	}

	var idLenRaw [2]byte
	if _, err := io.ReadFull(r, idLenRaw[:]); err != nil {
		return hello, nil, fmt.Errorf("read Auth v2 client id length: %w", err)
	}
	idLen := int(binary.BigEndian.Uint16(idLenRaw[:]))
	if idLen < 8 || idLen > maxClientIDLen {
		return hello, nil, fmt.Errorf("invalid tunnel authentication")
	}
	idRaw := make([]byte, idLen)
	if _, err := io.ReadFull(r, idRaw); err != nil {
		return hello, nil, fmt.Errorf("read Auth v2 client id: %w", err)
	}
	clientID := string(idRaw)

	var clientNonce [nonceSize]byte
	if _, err := io.ReadFull(r, clientNonce[:]); err != nil {
		return hello, nil, fmt.Errorf("read Auth v2 client nonce: %w", err)
	}

	credential, valid := registry.Resolve(clientID, now)
	secret := credential.Secret
	if !valid {
		// Continue the same challenge flow for unknown/disabled IDs so a remote
		// peer cannot cheaply distinguish a valid client id before proving a secret.
		fake := make([]byte, 32)
		if _, err := rand.Read(fake); err != nil {
			return hello, nil, err
		}
		secret = hex.EncodeToString(fake)
	}

	var serverNonce [nonceSize]byte
	if _, err := rand.Read(serverNonce[:]); err != nil {
		return hello, nil, fmt.Errorf("generate Auth v2 server nonce: %w", err)
	}
	if err := writeFull(w, serverNonce[:]); err != nil {
		return hello, nil, fmt.Errorf("send Auth v2 challenge: %w", err)
	}

	var encryptedLenRaw [2]byte
	if _, err := io.ReadFull(r, encryptedLenRaw[:]); err != nil {
		return hello, nil, fmt.Errorf("read Auth v2 proof length: %w", err)
	}
	encryptedLen := int(binary.BigEndian.Uint16(encryptedLenRaw[:]))
	if encryptedLen <= 0 || encryptedLen > maxHelloCipher {
		return hello, nil, fmt.Errorf("invalid tunnel authentication")
	}
	encrypted := make([]byte, encryptedLen)
	if _, err := io.ReadFull(r, encrypted); err != nil {
		return hello, nil, fmt.Errorf("read Auth v2 proof: %w", err)
	}

	authCipher, err := deriveV2Cipher(secret, clientNonce, serverNonce, "chameleon/auth/v2")
	if err != nil {
		return hello, nil, err
	}
	plain, err := authCipher.Open(encrypted)
	if err != nil || !valid {
		return hello, nil, fmt.Errorf("invalid tunnel authentication")
	}
	if len(plain) < 3 || plain[0] != authV2Version {
		return hello, nil, fmt.Errorf("invalid tunnel authentication")
	}
	destLen := int(binary.BigEndian.Uint16(plain[1:3]))
	if destLen <= 0 || destLen > maxDestinationLen || len(plain) != 3+destLen {
		return hello, nil, fmt.Errorf("invalid destination length")
	}
	destination := string(plain[3:])
	if err := validateDestination(destination); err != nil {
		return hello, nil, err
	}

	sessionCipher, err := deriveV2Cipher(secret, clientNonce, serverNonce, "chameleon/tunnel/v2/session")
	if err != nil {
		return hello, nil, err
	}
	hello.Version = authV2Version
	hello.Nonce = clientNonce
	hello.Destination = destination
	hello.ClientID = clientID
	return hello, sessionCipher, nil
}

func deriveV2Cipher(secret string, clientNonce, serverNonce [nonceSize]byte, info string) (*chcrypto.Cipher, error) {
	if err := validateClientSecret(secret); err != nil {
		return nil, err
	}
	salt := make([]byte, 0, nonceSize*2)
	salt = append(salt, clientNonce[:]...)
	salt = append(salt, serverNonce[:]...)
	reader := hkdf.New(sha256.New, []byte(secret), salt, []byte(info))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive Auth v2 key: %w", err)
	}
	return chcrypto.NewCipherFromKey(key)
}
