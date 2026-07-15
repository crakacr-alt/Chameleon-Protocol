package experiment

import (
	"testing"

	"github.com/Hack2p/chameleon/pkg/core"
)

func TestScenarioRunProducesMetrics(t *testing.T) {
	t.Parallel()

	scenario := Scenario{
		Profile:      core.ProfileWebRTC,
		Payload:      []byte("hello-chameleon"),
		Burst:        2,
		Rounds:       1,
		SharedSecret: "research-secret",
	}

	metrics, err := scenario.Run()
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if metrics.PacketsSent != 2 {
		t.Fatalf("unexpected packets sent: %d", metrics.PacketsSent)
	}
	if metrics.Profile != string(core.ProfileWebRTC) {
		t.Fatalf("unexpected profile: %s", metrics.Profile)
	}
}
