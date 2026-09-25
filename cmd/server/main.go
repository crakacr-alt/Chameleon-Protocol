package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"strings"

	"github.com/Hack2p/chameleon/pkg/core"
	chameleoncrypto "github.com/Hack2p/chameleon/pkg/crypto"
	idstore "github.com/Hack2p/chameleon/pkg/identity"
)

func normalizeListenAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ":9000"
	}
	if strings.Contains(address, ":") {
		return address
	}
	return address + ":9000"
}

func main() {
	address := flag.String("address", ":9000", "UDP address to listen on")
	psk := flag.String("psk", "research-secret", "legacy shared secret for unauthenticated compatibility mode")
	requireAuth := flag.Bool("require-auth", false, "reject data packets until an authenticated handshake succeeds")
	identityStorePath := flag.String("identity-store", "identity_store.json", "TOFU peer identity store")
	serverKeyPath := flag.String("server-key", "server.key", "persistent Ed25519 server identity")
	flag.Parse()

	listenAddress := normalizeListenAddress(*address)
	conn, err := net.ListenPacket("udp", listenAddress)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	legacyCipher, err := chameleoncrypto.NewCipher(*psk)
	if err != nil {
		panic(err)
	}

	store, err := idstore.NewStore(*identityStorePath)
	if err != nil {
		panic(err)
	}

	km, err := chameleoncrypto.NewKeyManager(*serverKeyPath)
	if err != nil {
		panic(err)
	}

	peerCiphers := make(map[string]*chameleoncrypto.Cipher)
	fmt.Printf("server listening on %s (require-auth=%t)\n", listenAddress, *requireAuth)

	buffer := make([]byte, 64*1024)
	for {
		n, remoteAddr, err := conn.ReadFrom(buffer)
		if err != nil {
			fmt.Println("read error:", err)
			continue
		}
		if n == 0 {
			continue
		}

		var message map[string]string
		if err := json.Unmarshal(buffer[:n], &message); err == nil {
			id := message["identity"]
			xpubb := message["x25519"]
			edpubb := message["ed25519"]
			sigb := message["sig"]

			if id != "" && xpubb != "" && edpubb != "" && sigb != "" {
				clientXpub, err := base64.StdEncoding.DecodeString(xpubb)
				if err != nil {
					fmt.Printf("invalid x25519 key from %s: %v\n", remoteAddr, err)
					continue
				}
				clientEdPub, err := base64.StdEncoding.DecodeString(edpubb)
				if err != nil {
					fmt.Printf("invalid ed25519 key from %s: %v\n", remoteAddr, err)
					continue
				}
				clientSig, err := base64.StdEncoding.DecodeString(sigb)
				if err != nil {
					fmt.Printf("invalid signature from %s: %v\n", remoteAddr, err)
					continue
				}

				if err := chameleoncrypto.VerifySignedPublic(clientXpub, clientEdPub, clientSig); err != nil {
					fmt.Printf("handshake signature failed from %s: %v\n", remoteAddr, err)
					continue
				}

				created, err := store.RegisterOrVerify(id, edpubb)
				if err != nil {
					fmt.Printf("identity verification failed for %s: %v\n", id, err)
					continue
				}

				serverHandshake, err := chameleoncrypto.NewAuthHandshakeWithKeyManager(km)
				if err != nil {
					fmt.Printf("create server handshake: %v\n", err)
					continue
				}

				sharedSecret, err := serverHandshake.DeriveSharedSecret(clientXpub, clientEdPub, clientSig)
				if err != nil {
					fmt.Printf("derive shared secret for %s: %v\n", id, err)
					continue
				}

				context := chameleoncrypto.SessionContext(clientXpub, serverHandshake.X25519Public())
				sessionKey, err := chameleoncrypto.DeriveSessionKey(sharedSecret, context, 32)
				if err != nil {
					fmt.Printf("derive session key for %s: %v\n", id, err)
					continue
				}

				sessionCipher, err := chameleoncrypto.NewCipherFromKey(sessionKey)
				if err != nil {
					fmt.Printf("create session cipher for %s: %v\n", id, err)
					continue
				}

				serverSig, err := km.Sign(serverHandshake.X25519Public())
				if err != nil {
					fmt.Printf("sign server handshake: %v\n", err)
					continue
				}

				response := map[string]string{
					"identity": "server",
					"x25519":   base64.StdEncoding.EncodeToString(serverHandshake.X25519Public()),
					"ed25519":  base64.StdEncoding.EncodeToString(km.Public()),
					"sig":      base64.StdEncoding.EncodeToString(serverSig),
				}
				data, err := json.Marshal(response)
				if err != nil {
					fmt.Printf("marshal server handshake: %v\n", err)
					continue
				}
				if _, err := conn.WriteTo(data, remoteAddr); err != nil {
					fmt.Printf("send server handshake: %v\n", err)
					continue
				}

				peerCiphers[remoteAddr.String()] = sessionCipher
				fmt.Printf("authenticated peer %s from %s (new-pin=%t)\n", id, remoteAddr, created)
				continue
			}
		}

		payload, err := core.DecodeFrame(buffer[:n])
		if err != nil {
			fmt.Printf("failed to decode frame from %s: %v\n", remoteAddr, err)
			continue
		}

		activeCipher := legacyCipher
		if sessionCipher, ok := peerCiphers[remoteAddr.String()]; ok {
			activeCipher = sessionCipher
		} else if *requireAuth {
			fmt.Printf("rejected unauthenticated data packet from %s\n", remoteAddr)
			continue
		}

		plain, err := activeCipher.Open(payload)
		if err != nil {
			fmt.Printf("failed to decrypt payload from %s: %v\n", remoteAddr, err)
			continue
		}

		fmt.Printf("received %d bytes from %s\n", len(plain), remoteAddr)
		if _, err := conn.WriteTo(buffer[:n], remoteAddr); err != nil {
			fmt.Println("write error:", err)
		}
	}
}
