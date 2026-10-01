package tunnel

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"time"

	chcrypto "github.com/crakacr-alt/Chameleon-Protocol/pkg/crypto"
)

// authenticateConn selects Auth v2 by its explicit preface; all other
// connections fall back to the legacy PSK hello while that compatibility
// credential remains configured.
func (s *Server) authenticateConn(conn net.Conn) (clientHello, *chcrypto.Cipher, error) {
	var hello clientHello
	if s == nil || conn == nil {
		return hello, nil, fmt.Errorf("invalid tunnel connection")
	}
	reader := bufio.NewReader(conn)
	prefix, err := reader.Peek(len(authV2Magic))
	if err != nil {
		return hello, nil, fmt.Errorf("read tunnel authentication preface: %w", err)
	}

	if bytes.Equal(prefix, authV2Magic) {
		if s.cfg.Clients == nil || s.cfg.Clients.Len() == 0 {
			return hello, nil, fmt.Errorf("invalid tunnel authentication")
		}
		return serverHandshakeV2(reader, conn, s.cfg.Clients, time.Now())
	}

	if stringsTrim(s.cfg.PSK) == "" {
		return hello, nil, fmt.Errorf("invalid tunnel authentication")
	}
	hello, err = readClientHello(reader, s.cfg.PSK, time.Now(), s.cfg.MaxClockSkew)
	if err != nil {
		return hello, nil, err
	}
	if !s.acceptNonce(hello.Nonce, time.Now()) {
		return hello, nil, fmt.Errorf("replayed tunnel hello")
	}
	cipher, err := deriveCipher(s.cfg.PSK, hello.Nonce)
	if err != nil {
		return hello, nil, err
	}
	return hello, cipher, nil
}
