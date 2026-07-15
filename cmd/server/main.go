package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/Hack2p/chameleon/pkg/core"
	chameleoncrypto "github.com/Hack2p/chameleon/pkg/crypto"
)

func main() {
	address := flag.String("address", ":9000", "UDP address to listen on")
	psk := flag.String("psk", "research-secret", "shared secret for optional AEAD decryption")
	flag.Parse()

	conn, err := net.ListenPacket("udp", *address)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	cipher, err := chameleoncrypto.NewCipher(*psk)
	if err != nil {
		panic(err)
	}

	fmt.Printf("server listening on %s\n", *address)

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
