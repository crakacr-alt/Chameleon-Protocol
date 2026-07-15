package experiment

import (
	"testing"

	"github.com/Hack2p/chameleon/pkg/core"
)

func TestCompareProfilesProducesMatrix(t *testing.T) {
	t.Parallel()

	matrix, err := CompareProfiles(Scenario{
		Profile:      core.ProfileWebRTC,
		Payload:      []byte("hello-chameleon"),
		Burst:        1,
		Rounds:       1,
		SharedSecret: "research-secret",
	}, Scenario{
		Profile:      core.ProfileHTTP3,
		Payload:      []byte("hello-chameleon"),
		Burst:        1,
		Rounds:       1,
		SharedSecret: "research-secret",
	})
	if err != nil {
		t.Fatalf("CompareProfiles returned error: %v", err)
	}

	if len(matrix) != 2 {
		t.Fatalf("unexpected matrix size: %d", len(matrix))
	}
	if _, ok := matrix[string(core.ProfileWebRTC)]; !ok {
		t.Fatal("expected webrtc result in comparison matrix")
	}
	if _, ok := matrix[string(core.ProfileHTTP3)]; !ok {
		t.Fatal("expected http3 result in comparison matrix")
	}
}
