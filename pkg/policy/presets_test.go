package policy

import "testing"

func TestNormalizePreset(t *testing.T) {
	for _, name := range Names() {
		got, err := Normalize(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) != name {
			t.Fatalf("%s: got %s", name, got)
		}
	}
	if _, err := Normalize("unknown"); err == nil {
		t.Fatal("expected invalid preset error")
	}
}

func TestPresetsChangeRealWeights(t *testing.T) {
	gaming, err := CarrierPolicy("gaming")
	if err != nil {
		t.Fatal(err)
	}
	streaming, err := CarrierPolicy("streaming")
	if err != nil {
		t.Fatal(err)
	}
	if gaming.LatencyScale <= streaming.LatencyScale {
		t.Fatal("gaming must value latency more than streaming")
	}
	if gaming.JitterScale <= streaming.JitterScale {
		t.Fatal("gaming must value jitter more than streaming")
	}
	if streaming.ThroughputScale <= gaming.ThroughputScale {
		t.Fatal("streaming must value throughput more than gaming")
	}
}
