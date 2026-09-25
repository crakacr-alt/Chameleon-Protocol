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


func TestAdaptiveDialerLearnsEarlyDirectDPIFailure(t *testing.T) {
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

	dialer := &AdaptiveDialer{
		Planner:       p,
		Carriers:      []carrier.Candidate{{Name: "direct", Kind: carrier.KindDirect, SupportsTCP: true}},
		DPIStrategies: dpi.DefaultStrategies(),
		Timeout:       time.Second,
		Network: func() (networkctx.Context, error) {
			return networkctx.Context{ID: "mobile-reset"}, nil
		},
		DirectDial: func(context.Context, string) (net.Conn, error) {
			clientSide, serverSide := net.Pipe()
			go func() {
				buf := make([]byte, 64)
				_, _ = serverSide.Read(buf)
				_ = serverSide.Close()
			}()
			return clientSide, nil
		},
	}

	conn, err := dialer.DialContext(context.Background(), "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("client-hello-like-data")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("expected early terminal read error")
	}
	_ = conn.Close()

	dpiCtx := dpi.Context{
		NetworkID:    "mobile-reset",
		Destination:  "example.com:443",
		TrafficClass: "web",
	}
	stats := dpiEngine.Snapshot(dpiCtx)["direct"]
	if stats.Failures != 1 {
		t.Fatalf("expected one learned DPI failure, got %+v", stats)
	}

	carrierCtx := carrier.Context{
		NetworkID:    "mobile-reset",
		Destination:  "example.com:443",
		TrafficClass: "web",
		Protocol:     "tcp",
	}
	carrierStats := carrierEngine.Snapshot(carrierCtx)["direct"]
	if carrierStats.Successes != 1 {
		t.Fatalf("DPI failure must not duplicate carrier success, got %+v", carrierStats)
	}

	next, err := p.Choose(planner.Request{
		Network:       networkctx.Context{ID: "mobile-reset"},
		Destination:   "example.com:443",
		Protocol:      "tcp",
		Purpose:       "web",
		Carriers:      dialer.Carriers,
		DPIStrategies: dialer.DPIStrategies,
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.DPI.Strategy.Name != "split-early" {
		t.Fatalf("expected split-early after direct failure, got %q", next.DPI.Strategy.Name)
	}
}

func TestAdaptiveDialerSkipsDirectWhenAllDPIStrategiesRecentlyFailed(t *testing.T) {
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

	strategies := dpi.DefaultStrategies()
	ctx := dpi.Context{
		NetworkID:    "blocked-mobile",
		Destination:  "example.com:443",
		TrafficClass: "web",
	}
	for _, strategy := range strategies {
		if err := dpiEngine.Observe(dpi.Observation{
			Context:  ctx,
			Strategy: strategy.Name,
			Success:  false,
			Failure:  "early reset",
			At:       time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	var firstDial string
	dialer := &AdaptiveDialer{
		Planner:       p,
		Carriers:      carrier.Defaults("", "", "127.0.0.1:1081"),
		DPIStrategies: strategies,
		Timeout:       time.Second,
		Network: func() (networkctx.Context, error) {
			return networkctx.Context{ID: "blocked-mobile"}, nil
		},
		DirectDial: func(_ context.Context, address string) (net.Conn, error) {
			if firstDial == "" {
				firstDial = address
			}
			return nil, io.ErrClosedPipe
		},
	}

	_, _ = dialer.DialContext(context.Background(), "example.com:443")
	if firstDial != "127.0.0.1:1081" {
		t.Fatalf("expected direct cooldown to start with relay endpoint, got %q", firstDial)
	}
}


func TestAdaptiveDialerLearnsDirectBlackholeFromFirstResponseTimeout(t *testing.T) {
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

	dialer := &AdaptiveDialer{
		Planner:                  p,
		Carriers:                 []carrier.Candidate{{Name: "direct", Kind: carrier.KindDirect, SupportsTCP: true}},
		DPIStrategies:            dpi.DefaultStrategies(),
		Timeout:                  time.Second,
		ApplicationFailureWindow: 40 * time.Millisecond,
		Network: func() (networkctx.Context, error) {
			return networkctx.Context{ID: "mobile-blackhole"}, nil
		},
		DirectDial: func(context.Context, string) (net.Conn, error) {
			clientSide, serverSide := net.Pipe()
			go func() {
				buf := make([]byte, 64)
				_, _ = serverSide.Read(buf)
				time.Sleep(200 * time.Millisecond)
				_ = serverSide.Close()
			}()
			return clientSide, nil
		},
	}

	conn, err := dialer.DialContext(context.Background(), "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("client-hello-like-data")); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("expected first-response timeout")
	}
	_ = conn.Close()

	stats := dpiEngine.Snapshot(dpi.Context{
		NetworkID:    "mobile-blackhole",
		Destination:  "example.com:443",
		TrafficClass: "web",
	})["direct"]
	if stats.Failures != 1 {
		t.Fatalf("expected blackhole to be learned as DPI failure, got %+v", stats)
	}
}
