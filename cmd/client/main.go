package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/Hack2p/chameleon/pkg/core"
	chcrypto "github.com/Hack2p/chameleon/pkg/crypto"
	idstore "github.com/Hack2p/chameleon/pkg/identity"
)

func main() {
	target := flag.String("target", "127.0.0.1:9000", "UDP endpoint to send to")
	payload := flag.String("payload", "hello-chameleon", "payload to normalize")
	profile := flag.String("profile", string(core.ProfileWebRTC), "traffic profile: webrtc, http3, gaming")
	burst := flag.Int("burst", 1, "number of shaped packets to send")
	psk := flag.String("psk", "research-secret", "legacy shared secret used when authenticated handshake is disabled")
	adaptiveStorePath := flag.String("adaptive-store", "", "optional path to a JSON learner state file")
	sessionMemoryPath := flag.String("session-memory", "", "optional path to a JSON session-memory file")
	identity := flag.String("identity", "", "persistent client identity name")
	idStorePath := flag.String("id-store", "identity.json", "TOFU identity store path")
	sendHandshake := flag.Bool("send-handshake", false, "perform authenticated X25519+Ed25519 session bootstrap")
	flag.Parse()

	var (
		handshake *chcrypto.AuthHandshake
		store     *idstore.Store
	)

	if *identity != "" {
		var err error
		store, err = idstore.NewStore(*idStorePath)
		if err != nil {
			panic(err)
		}

		keyPath := filepath.Join(filepath.Dir(*idStorePath), *identity+".key")
		km, err := chcrypto.NewKeyManager(keyPath)
		if err != nil {
			panic(err)
		}

		handshake, err = chcrypto.NewAuthHandshakeWithKeyManager(km)
		if err != nil {
			panic(err)
		}

		clientPub := base64.StdEncoding.EncodeToString(km.Public())
		if _, err := store.RegisterOrVerify(*identity, clientPub); err != nil {
			panic(err)
		}

		fmt.Printf("loaded identity %s (key=%s)\n", *identity, keyPath)
	}

	conn, err := net.Dial("udp", *target)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	var sessionCipher *chcrypto.Cipher

	if *sendHandshake {
		if handshake == nil || store == nil {
			fmt.Fprintln(os.Stderr, "--send-handshake requires --identity")
			os.Exit(2)
		}

		clientXpub := handshake.X25519Public()
		clientSig, err := handshake.SignX25519()
		if err != nil {
			panic(err)
		}

		message := map[string]string{
			"identity": *identity,
			"x25519":   base64.StdEncoding.EncodeToString(clientXpub),
			"ed25519":  base64.StdEncoding.EncodeToString(handshake.Ed25519Public()),
			"sig":      base64.StdEncoding.EncodeToString(clientSig),
		}

		data, err := json.Marshal(message)
		if err != nil {
			panic(err)
		}

		if _, err := conn.Write(data); err != nil {
			panic(err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			panic(err)
		}

		response := make([]byte, 4096)
		n, err := conn.Read(response)
		if err != nil {
			panic(fmt.Errorf("read server handshake: %w", err))
		}
		_ = conn.SetReadDeadline(time.Time{})

		var serverMessage map[string]string
		if err := json.Unmarshal(response[:n], &serverMessage); err != nil {
			panic(fmt.Errorf("decode server handshake: %w", err))
		}
		if serverMessage["identity"] != "server" {
			panic("unexpected server identity")
		}

		serverXpub, err := base64.StdEncoding.DecodeString(serverMessage["x25519"])
		if err != nil {
			panic(fmt.Errorf("decode server x25519 key: %w", err))
		}
		serverEdPub, err := base64.StdEncoding.DecodeString(serverMessage["ed25519"])
		if err != nil {
			panic(fmt.Errorf("decode server ed25519 key: %w", err))
		}
		serverSig, err := base64.StdEncoding.DecodeString(serverMessage["sig"])
		if err != nil {
			panic(fmt.Errorf("decode server signature: %w", err))
		}

		if _, err := store.RegisterOrVerify("server", serverMessage["ed25519"]); err != nil {
			panic(fmt.Errorf("server identity pin check failed: %w", err))
		}

		sharedSecret, err := handshake.DeriveSharedSecret(serverXpub, serverEdPub, serverSig)
		if err != nil {
			panic(fmt.Errorf("authenticated key exchange failed: %w", err))
		}

		context := chcrypto.SessionContext(clientXpub, serverXpub)
		sessionKey, err := chcrypto.DeriveSessionKey(sharedSecret, context, 32)
		if err != nil {
			panic(fmt.Errorf("derive session key: %w", err))
		}

		sessionCipher, err = chcrypto.NewCipherFromKey(sessionKey)
		if err != nil {
			panic(fmt.Errorf("create session cipher: %w", err))
		}

		fmt.Printf("authenticated session established with %s\n", *target)
	}

	transport, err := core.NewTransport(conn, core.Config{
		Profile:           core.BehaviorProfile(*profile),
		SharedSecret:      *psk,
		AdaptiveStorePath: *adaptiveStorePath,
		SessionMemoryPath: *sessionMemoryPath,
	})
	if err != nil {
		panic(err)
	}

	if sessionCipher != nil {
		transport.UpdateCipher(sessionCipher)
	}

	if err := transport.SendBurst([]byte(*payload), *burst); err != nil {
		panic(err)
	}

	fmt.Printf("sent %d shaped packet(s) to %s using profile %s\n", *burst, *target, *profile)
}
