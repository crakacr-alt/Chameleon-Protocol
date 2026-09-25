package proxy

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
)

func TestAdaptiveDialerLearnsCarrierThenDPI(t *testing.T) {
	carrierEngine, err := carrier.NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	dpiEngine, err := dpi.NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	p, err := planner.New(carrierEngine, dpiEngine)
	if err != nil {
		t.Fatal(err)
	}

	clientSide, echoSide := net.Pipe()
	go func() {
		_, _ = io.Copy(echoSide, echoSide)
		_ = echoSide.Close()
	}()

	dialer := &AdaptiveDialer{
		Planner:       p,
		Carriers:      []carrier.Candidate{{Name: "direct", Kind: carrier.KindDirect, SupportsTCP: true}},
		DPIStrategies: dpi.DefaultStrategies(),
		Timeout:       time.Second,
		Network: func() (networkctx.Context, error) {
			return networkctx.Context{ID: "test-net"}, nil
		},
		DirectDial: func(context.Context, string) (net.Conn, error) {
			return clientSide, nil
		},
	}

	conn, err := dialer.DialContext(context.Background(), "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 5)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "hello" {
		t.Fatalf("unexpected reply %q", reply)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	carrierStats := carrierEngine.Snapshot(carrier.Context{
		NetworkID:    "test-net",
		Destination:  "example.com:443",
		TrafficClass: "web",
		Protocol:     "tcp",
	})
	if carrierStats["direct"].Successes != 1 {
		t.Fatalf("carrier success not learned: %+v", carrierStats)
	}

	dpiStats := dpiEngine.Snapshot(dpi.Context{
		NetworkID:    "test-net",
		Destination:  "example.com:443",
		TrafficClass: "web",
	})
	if dpiStats["direct"].Successes != 1 {
		t.Fatalf("DPI success not learned after response bytes: %+v", dpiStats)
	}
}
