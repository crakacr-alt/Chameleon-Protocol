package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/Hack2p/chameleon/pkg/core"
)

func main() {
	target := flag.String("target", "127.0.0.1:9000", "UDP endpoint to send to")
	payload := flag.String("payload", "hello-chameleon", "payload to normalize")
	profile := flag.String("profile", string(core.ProfileWebRTC), "traffic profile: webrtc, http3, gaming")
	burst := flag.Int("burst", 1, "number of shaped packets to send")
	psk := flag.String("psk", "research-secret", "shared secret for optional AEAD encryption")
	adaptiveStorePath := flag.String("adaptive-store", "", "optional path to a JSON learner state file")
	sessionMemoryPath := flag.String("session-memory", "", "optional path to a JSON session-memory file")
	flag.Parse()

	conn, err := net.Dial("udp", *target)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

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
