package dpi

import (
	"bytes"
	"errors"
	"testing"
)

type recordingWriter struct {
	writes [][]byte
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	cp := append([]byte(nil), p...)
	w.writes = append(w.writes, cp)
	return len(p), nil
}

func TestApplyWriterDirect(t *testing.T) {
	w := &recordingWriter{}
	strategy := DefaultStrategies()[0]

	if err := ApplyWriter(w, []byte("abcdef"), strategy); err != nil {
		t.Fatal(err)
	}
	if len(w.writes) != 1 || !bytes.Equal(w.writes[0], []byte("abcdef")) {
		t.Fatalf("unexpected writes: %q", w.writes)
	}
}

func TestApplyWriterSplit(t *testing.T) {
	w := &recordingWriter{}
	strategy := Strategy{
		Name:        "test-split",
		Techniques:  []Technique{TechniqueSplit},
		SplitPoints: []int{1, 3},
	}

	if err := ApplyWriter(w, []byte("abcdef"), strategy); err != nil {
		t.Fatal(err)
	}

	want := [][]byte{[]byte("a"), []byte("bc"), []byte("def")}
	if len(w.writes) != len(want) {
		t.Fatalf("expected %d writes, got %d", len(want), len(w.writes))
	}
	for i := range want {
		if !bytes.Equal(w.writes[i], want[i]) {
			t.Fatalf("write %d: want %q, got %q", i, want[i], w.writes[i])
		}
	}
}

func TestApplyWriterRejectsPacketControlStrategy(t *testing.T) {
	w := &recordingWriter{}
	strategy := PacketControlStrategies()[0]

	err := ApplyWriter(w, []byte("abcdef"), strategy)
	if !errors.Is(err, ErrRequiresPacketControl) {
		t.Fatalf("expected ErrRequiresPacketControl, got %v", err)
	}
}
