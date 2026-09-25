package traffic

import (
	"net"
	"strconv"
	"strings"
)

// Class is used by the adaptive planner because different traffic needs
// different trade-offs. Games care about latency; bulk traffic cares more
// about throughput.
type Class string

const (
	ClassDefault     Class = "default"
	ClassWeb         Class = "web"
	ClassInteractive Class = "interactive"
	ClassStreaming   Class = "streaming"
	ClassBulk        Class = "bulk"
	ClassRealtime    Class = "realtime"
)

// Hint contains information already known by the caller.
// The classifier does not inspect encrypted application payload.
type Hint struct {
	Network     string
	Destination string
	Protocol    string
	Purpose     string
}

// Classify returns a conservative traffic class.
func Classify(h Hint) Class {
	purpose := strings.ToLower(strings.TrimSpace(h.Purpose))
	switch purpose {
	case "game", "gaming", "interactive", "ssh", "terminal":
		return ClassInteractive
	case "video", "stream", "streaming", "call", "voice":
		return ClassStreaming
	case "download", "upload", "backup", "sync", "bulk":
		return ClassBulk
	case "realtime", "real-time":
		return ClassRealtime
	case "web", "browser":
		return ClassWeb
	}

	port := destinationPort(h.Destination)
	proto := strings.ToLower(strings.TrimSpace(h.Protocol))

	if port == 80 || port == 443 {
		return ClassWeb
	}
	if port == 22 || port == 3389 {
		return ClassInteractive
	}
	if proto == "udp" && port != 53 {
		return ClassRealtime
	}
	return ClassDefault
}

func destinationPort(dst string) int {
	_, portText, err := net.SplitHostPort(dst)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0
	}
	return port
}
