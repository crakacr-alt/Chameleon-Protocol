package experiment

import (
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/Hack2p/chameleon/pkg/core"
)

// Metrics collects observable transport characteristics for a benchmark run.
type Metrics struct {
	Profile       string
	Rounds        int
	PacketsSent   int
	BytesSent     int64
	BytesReceived int64
	MeanLatency   time.Duration
	Throughput    float64
	LossRate      float64
	Entropy       float64
}

// Scenario defines one reproducible experiment.
type Scenario struct {
	Profile      core.BehaviorProfile
	Payload      []byte
	Burst        int
	Rounds       int
	SharedSecret string
}

// Run executes a local benchmark scenario and returns the metrics snapshot.
func (s Scenario) Run() (*Metrics, error) {
	if s.Burst <= 0 {
		return nil, fmt.Errorf("burst must be positive")
	}
	if s.Rounds <= 0 {
		return nil, fmt.Errorf("rounds must be positive")
	}
	if len(s.Payload) == 0 {
		return nil, fmt.Errorf("payload must not be empty")
	}

	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen udp: %w", err)
	}
	defer listener.Close()

	clientConn, err := net.Dial("udp", listener.LocalAddr().String())
	if err != nil {
		return nil, fmt.Errorf("dial udp: %w", err)
	}
	defer clientConn.Close()

	transport, err := core.NewTransport(clientConn, core.Config{
		Profile:      s.Profile,
		SharedSecret: s.SharedSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}

	packetsToExpect := int64(s.Burst * s.Rounds)
	var received int64
	var receivedBytes int64
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, _, err := listener.ReadFrom(buf)
			if err != nil {
				return
			}
			mu.Lock()
			received++
			receivedBytes += int64(n)
			if received >= packetsToExpect {
				mu.Unlock()
				return
			}
			mu.Unlock()
		}
	}()

	start := time.Now()
	latencyTotal := time.Duration(0)
	packetsSent := 0
	bytesSent := int64(0)

	for i := 0; i < s.Rounds; i++ {
		roundStart := time.Now()
		if err := transport.SendBurst(s.Payload, s.Burst); err != nil {
			return nil, fmt.Errorf("send burst: %w", err)
		}
		latencyTotal += time.Since(roundStart)
		packetsSent += s.Burst
		bytesSent += int64(len(s.Payload) * s.Burst)
	}

	elapsed := time.Since(start)
	if elapsed <= 0 {
		elapsed = time.Nanosecond
	}

	<-done

	mu.Lock()
	finalReceived := received
	finalBytesReceived := receivedBytes
	mu.Unlock()

	lossRate := 0.0
	if packetsSent > 0 {
		lossRate = float64(int64(packetsSent)-finalReceived) / float64(packetsSent)
	}

	meanLatency := time.Duration(0)
	if s.Rounds > 0 {
		meanLatency = latencyTotal / time.Duration(s.Rounds)
	}

	metrics := &Metrics{
		Profile:       string(s.Profile),
		Rounds:        s.Rounds,
		PacketsSent:   packetsSent,
		BytesSent:     bytesSent,
		BytesReceived: finalBytesReceived,
		MeanLatency:   meanLatency,
		Throughput:    float64(bytesSent) / elapsed.Seconds(),
		LossRate:      lossRate,
		Entropy:       entropy(s.Payload),
	}

	return metrics, nil
}

// Report prints a compact benchmark summary.
func (m *Metrics) Report() string {
	return fmt.Sprintf("profile=%s rounds=%d packets=%d bytes_sent=%d bytes_received=%d mean_latency=%s throughput=%.2f B/s loss_rate=%.4f entropy=%.4f",
		m.Profile,
		m.Rounds,
		m.PacketsSent,
		m.BytesSent,
		m.BytesReceived,
		m.MeanLatency,
		m.Throughput,
		m.LossRate,
		m.Entropy,
	)
}

func entropy(data []byte) float64 {
	counts := make(map[byte]int)
	for _, b := range data {
		counts[b]++
	}

	total := len(data)
	if total == 0 {
		return 0
	}

	entropy := 0.0
	for _, count := range counts {
		p := float64(count) / float64(total)
		entropy -= p * math.Log2(p)
	}

	return entropy
}
