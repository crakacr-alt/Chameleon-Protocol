package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Hack2p/chameleon/pkg/core"
	chameleoncrypto "github.com/Hack2p/chameleon/pkg/crypto"
	idstore "github.com/Hack2p/chameleon/pkg/identity"
	state "github.com/Hack2p/chameleon/pkg/state"
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
	psk := flag.String("psk", "research-secret", "shared secret for optional AEAD decryption")
	flag.Parse()

	listenAddress := normalizeListenAddress(*address)
	conn, err := net.ListenPacket("udp", listenAddress)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	cipher, err := chameleoncrypto.NewCipher(*psk)
	if err != nil {
		panic(err)
	}

	// prepare identity store for server-side registration of peers
	idStorePath := "identity_store.json"
	store, err := idstore.NewStore(idStorePath)
	if err != nil {
		fmt.Printf("warning: cannot open identity store %s: %v\n", idStorePath, err)
		store = nil
	}

	syncer := state.NewSync(*psk)
	syncer.EpochPeriod = 30 * time.Second

	// server key manager: persistent identity
	keyPath := "server.key"
	km, err := chameleoncrypto.NewKeyManager(keyPath)
	if err != nil {
		panic(err)
	}
	// create server-auth handshake that uses persistent identity from KeyManager
	serverAH, err := chameleoncrypto.NewAuthHandshakeWithKeyManager(km)
	if err != nil {
		// fallback to ephemeral
		serverAH, _ = chameleoncrypto.NewAuthHandshake()
	}

	fmt.Printf("epoch-based profile: %s\n", syncer.ProfileAt(time.Now()))

	fmt.Printf("server listening on %s\n", listenAddress)

	buffer := make([]byte, 4096)
	for {
		n, remoteAddr, err := conn.ReadFrom(buffer)
		if err != nil {
			fmt.Println("read error:", err)
			continue
		}
		if n < 4 {
			continue
		}

		// try to parse handshake JSON first
		var msg map[string]string
		if err := json.Unmarshal(buffer[:n], &msg); err == nil {
			// expected keys: identity, x25519, ed25519, sig
			id := msg["identity"]
			xpubb := msg["x25519"]
			edpubb := msg["ed25519"]
			sigb := msg["sig"]
			if id != "" && xpubb != "" && edpubb != "" && sigb != "" {
				clientXpub, err1 := base64.StdEncoding.DecodeString(xpubb)
				clientEdPub, err2 := base64.StdEncoding.DecodeString(edpubb)
				clientSig, err3 := base64.StdEncoding.DecodeString(sigb)
				if err1 == nil && err2 == nil && err3 == nil {
					// verify signature
					if err := chameleoncrypto.VerifySignedPublic(clientXpub, clientEdPub, clientSig); err != nil {
						fmt.Printf("handshake signature failed from %s: %v\n", remoteAddr.String(), err)
					} else {
						fmt.Printf("handshake verified from %s identity=%s\n", remoteAddr.String(), id)
						// register identity locally if store available
						if store != nil {
							if err := store.Register(id, base64.StdEncoding.EncodeToString(clientEdPub)); err != nil {
								fmt.Printf("failed to register identity %s: %v\n", id, err)
							}
						}
						// derive shared secret via KeyManager-derived epoch key
						// for demo: derive a symmetric key for current epoch using km
						secret, err := km.GetEpochKey([]byte("demo-epoch"), 32)
						if err != nil {
							fmt.Printf("derive shared secret failed: %v\n", err)
						} else {
							fmt.Printf("derived shared secret (%d bytes) for %s\n", len(secret), id)
						}

						// respond with server handshake (signed ed25519 public)
						sx := base64.StdEncoding.EncodeToString(serverAH.X25519Public())
						sed := base64.StdEncoding.EncodeToString(km.Public())
						ssig, _ := km.Sign(serverAH.X25519Public())
						ssigb := base64.StdEncoding.EncodeToString(ssig)
						resp := map[string]string{"identity": "server", "x25519": sx, "ed25519": sed, "sig": ssigb}
						if data, err := json.Marshal(resp); err == nil {
							if _, err := conn.WriteTo(data, remoteAddr); err != nil {
								fmt.Printf("failed to send server handshake: %v\n", err)
							}
						}

						// Wait for a rekey message from client and respond with server's rekey
						// (synchronous demo): read once and treat as rekey request
						rb := make([]byte, 2048)
						conn.SetReadDeadline(time.Now().Add(2 * time.Second))
						n2, _, err := conn.ReadFrom(rb)
						if err == nil && n2 > 0 {
							var req chameleoncrypto.RekeyMessage
							if err := json.Unmarshal(rb[:n2], &req); err == nil {
								// verify client request using registered identity if available
								if id != "" {
									// look up client ed25519 pub in store
									if store != nil {
										if spub64, ok := store.Lookup(id); ok {
											spub, _ := base64.StdEncoding.DecodeString(spub64)
											epoch, verr := chameleoncrypto.VerifyRekeyMessage(&req, spub)
											if verr == nil {
												// server derives symmetric key for epoch and responds with its rekey
												sk, _ := km.GetEpochKey(epoch, 32)
												// create server rekey message signed by server identity
												srvReq, _ := chameleoncrypto.CreateRekeyMessage(km, epoch, "server-info")
												if sdata, err := json.Marshal(srvReq); err == nil {
													if _, err := conn.WriteTo(sdata, remoteAddr); err != nil {
														fmt.Printf("failed to send server rekey: %v\n", err)
													} else {
														// wait for client's ack
														rb2 := make([]byte, 2048)
														conn.SetReadDeadline(time.Now().Add(2 * time.Second))
														n3, _, err := conn.ReadFrom(rb2)
														if err == nil && n3 > 0 {
															var ack chameleoncrypto.RekeyAck
															if err := json.Unmarshal(rb2[:n3], &ack); err == nil {
																// verify ack
																if _, verr := chameleoncrypto.VerifyRekeyAck(&ack, spub); verr == nil {
																	fmt.Printf("completed rekey exchange with %s and received ack\n", id)
																	// apply symmetric key swap here (server-side)
																	// e.g., find transport for remoteAddr and call UpdateCipher(newCipher)
																	_ = sk
																}
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
						continue
					}
				}
			}
		}

		payload, err := core.DecodeFrame(buffer[:n])
		if err != nil {
			fmt.Printf("failed to decode frame from %s: %v\n", remoteAddr.String(), err)
			continue
		}

		plain, err := cipher.Open(payload)
		if err != nil {
			fmt.Printf("failed to decrypt payload from %s: %v\n", remoteAddr.String(), err)
			continue
		}

		fmt.Printf("received %d bytes from %s: %s\n", len(plain), remoteAddr.String(), string(plain))
		if _, err := conn.WriteTo(buffer[:n], remoteAddr); err != nil {
			fmt.Println("write error:", err)
		}
	}
}
