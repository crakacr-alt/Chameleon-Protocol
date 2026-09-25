package dpi

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDirectIsDefaultWithoutFailure(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Best("wifi", "example.com", ProtocolTLS, FailureNone); got != StrategyDirect {
		t.Fatalf("expected direct, got %s", got)
	}
}

func TestTLSFailurePrefersCheapTLSWorkaround(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	got := e.Best("mobile", "example.com", ProtocolTLS, FailureTLSHandshake)
	if got != StrategyTLSRecordSplit && got != StrategyStreamSplit {
		t.Fatalf("unexpected first TLS workaround: %s", got)
	}
}

func TestSuccessfulStrategyIsRememberedPerNetwork(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := e.Observe(Result{
			NetworkID: "mobile-a", Destination: "example.com", Protocol: ProtocolTLS,
			Strategy: StrategyStreamSplit, Success: true, Latency: 40 * time.Millisecond,
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.Observe(Result{
			NetworkID: "mobile-a", Destination: "example.com", Protocol: ProtocolTLS,
			Strategy: StrategyDirect, Success: false, Failure: FailureReset, Latency: 2 * time.Second,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if got := e.Best("mobile-a", "example.com", ProtocolTLS, FailureNone); got != StrategyStreamSplit {
		t.Fatalf("expected learned stream split, got %s", got)
	}
	if got := e.Best("wifi", "example.com", ProtocolTLS, FailureNone); got != StrategyDirect {
		t.Fatalf("other network should remain direct, got %s", got)
	}
}

func TestQUICDoesNotReceiveTCPOnlyStrategies(t *testing.T) {
	e, err := NewEngine("")
	if err != nil {
		t.Fatal(err)
	}
	plan := e.Plan("n", "d", ProtocolQUIC, FailureUDPBlackhole)
	for _, item := range plan {
		switch item.Spec.Name {
		case StrategyStreamSplit, StrategyTLSRecordSplit, StrategyDisorder, StrategyFakePacket:
			t.Fatalf("TCP/TLS strategy leaked into QUIC plan: %s", item.Spec.Name)
		}
	}
}

func TestEnginePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dpi.json")
	e, err := NewEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Observe(Result{
		NetworkID: "lte", Destination: "site", Protocol: ProtocolTLS,
		Strategy: StrategyTLSRecordSplit, Success: true, Latency: 55 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Best("lte", "site", ProtocolTLS, FailureTLSHandshake); got != StrategyTLSRecordSplit {
		t.Fatalf("persisted strategy not restored, got %s", got)
	}
}
