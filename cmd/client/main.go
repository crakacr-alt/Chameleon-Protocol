package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
	"path/filepath"

	"github.com/Hack2p/chameleon/pkg/core"
	chcrypto "github.com/Hack2p/chameleon/pkg/crypto"
	idstore "github.com/Hack2p/chameleon/pkg/identity"
)

func main() {
	target := flag.String("target", "127.0.0.1:9000", "UDP endpoint to send to")
	payload := flag.String("payload", "hello-chameleon", "payload to normalize")
	profile := flag.String("profile", string(core.ProfileWebRTC), "traffic profile: webrtc, http3, gaming")
	burst := flag.Int("burst", 1, "number of shaped packets to send")
	psk := flag.String("psk", "research-secret", "shared secret for optional AEAD encryption")
	adaptiveStorePath := flag.String("adaptive-store", "", "optional path to a JSON learner state file")
	sessionMemoryPath := flag.String("session-memory", "", "optional path to a JSON session-memory file")

	// new flags for demoing authenticated handshake and identity registry
	identity := flag.String("identity", "", "optional identity name to register")
	idStorePath := flag.String("id-store", "identity.json", "path to local identity store (JSON)")
	sendHandshake := flag.Bool("send-handshake", false, "if true, send signed X25519 public to target as JSON")

	flag.Parse()

	// If identity is provided, create or load persistent Ed25519 key via KeyManager
	var ah *chcrypto.AuthHandshake
	if *identity != "" {
		// ensure id store exists
		store, err := idstore.NewStore(*idStorePath)
		if err != nil {
			panic(err)
		}
		// determine key path next to the id store (e.g., identity.key)
		keyPath := "" 
		if *idStorePath == "" {
			keyPath = *identity + ".key"
		} else {
			keyPath = filepath.Join(filepath.Dir(*idStorePath), *identity+".key")
		}
		km, err := chcrypto.NewKeyManager(keyPath)
		if err != nil {
			panic(err)
		}
		ah, err = chcrypto.NewAuthHandshakeWithKeyManager(km)
		if err != nil {
			panic(err)
		}
		edpub := base64.StdEncoding.EncodeToString(km.Public())
		if err := store.Register(*identity, edpub); err != nil {
			// non-fatal: continue
			fmt.Printf("warning: register identity: %v\n", err)
		}
		fmt.Printf("loaded identity %s pub=%s (key=%s)\n", *identity, edpub, keyPath)
	}

	conn, err := net.Dial("udp", *target)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	if *sendHandshake {
		if ah == nil {
			fmt.Fprintln(os.Stderr, "send-handshake requires --identity to be set")
			os.Exit(1)
		}
		// prepare handshake message
		xpub := base64.StdEncoding.EncodeToString(ah.X25519Public())
		edpub := base64.StdEncoding.EncodeToString(ah.Ed25519Public())
		sig, err := ah.SignX25519()
		if err != nil {
			panic(err)
		}
		sigb := base64.StdEncoding.EncodeToString(sig)

		msg := map[string]string{
			"identity": *identity,
			"x25519":   xpub,
			"ed25519":  edpub,
			"sig":      sigb,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			panic(err)
		}
		if _, err := conn.Write(data); err != nil {
			panic(err)
		}
		fmt.Printf("sent signed handshake to %s\n", *target)

		// wait short time for server response (synchronous demo)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		resp := make([]byte, 2048)
		n, err := conn.Read(resp)
		if err == nil && n > 0 {
			var r map[string]string
			if err := json.Unmarshal(resp[:n], &r); err == nil {
				fmt.Printf("received server handshake: %+v\n", r)
				// if server provided ed25519 pub, we can start rekey
				if r["ed25519"] != "" {
					// prepare rekey message to agree on epoch key
					km, err := chcrypto.NewKeyManager(*idStorePath + "/client.key")
					if err != nil {
						fmt.Printf("keymanager error: %v\n", err)
					} else {
						epochID := []byte("epoch-" + fmt.Sprint(time.Now().Unix()/30))
						rekeyMsg, _ := chcrypto.CreateRekeyMessage(km, epochID, "info")
						rm, _ := json.Marshal(rekeyMsg)
						if _, err := conn.Write(rm); err == nil {
							fmt.Printf("sent rekey message to %s\n", *target)
						}
							// read server rekey response
							n2, err := conn.Read(resp)
							if err == nil && n2 > 0 {
								var rmResp chcrypto.RekeyMessage
								if err := json.Unmarshal(resp[:n2], &rmResp); err == nil {
									peerPub, _ := base64.StdEncoding.DecodeString(r["ed25519"])
									epoch, verr := chcrypto.VerifyRekeyMessage(&rmResp, peerPub)
									if verr == nil {
										// derive symmetric key locally using KeyManager
										sk, _ := km.GetEpochKey(epoch, 32)
										aead, _ := chcrypto.NewCipherFromKey(sk)
										// send ack back to server to confirm installation
										ack, _ := chcrypto.CreateRekeyAck(km, epoch, km.Public())
										ackb, _ := json.Marshal(ack)
										if _, err := conn.Write(ackb); err == nil {
											fmt.Printf("sent rekey ack to server and installed new key\n")
											// here we would swap transport cipher atomically
											_ = aead
										}
									}
								}
							}
					}
				}
			}
		}
	}

	// legacy shaped send
	transport, err := core.NewTransport(conn, core.Config{
		Profile:           core.BehaviorProfile(*profile),
		SharedSecret:      *psk,
		AdaptiveStorePath: *adaptiveStorePath,
		SessionMemoryPath: *sessionMemoryPath,
	})
	if err != nil {
		panic(err)
	}

	if err := transport.SendBurst([]byte(*payload), *burst); err != nil {
		panic(err)
	}

	fmt.Printf("sent %d shaped packet(s) to %s using profile %s\n", *burst, *target, *profile)
}
