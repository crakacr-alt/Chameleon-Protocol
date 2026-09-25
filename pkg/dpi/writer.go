package dpi

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

// ErrRequiresPacketControl means the selected strategy needs a platform
// backend with packet-level access. The generic writer must not pretend
// that a normal TCP write can emulate raw packet semantics.
var ErrRequiresPacketControl = errors.New("strategy requires packet-level control")

// ApplyWriter runs a cross-platform userspace strategy on a byte stream.
// It is useful for a local proxy or an embedded client without root access.
func ApplyWriter(w io.Writer, payload []byte, strategy Strategy) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}
	if len(payload) == 0 {
		return fmt.Errorf("payload must not be empty")
	}
	if err := strategy.Validate(); err != nil {
		return err
	}
	if strategy.RequiresPacketControl {
		return ErrRequiresPacketControl
	}

	points := normalizedSplitPoints(strategy.SplitPoints, len(payload))
	if len(points) == 0 || onlyDirect(strategy.Techniques) {
		return writeAll(w, payload)
	}

	start := 0
	for _, end := range append(points, len(payload)) {
		if end <= start {
			continue
		}
		if err := writeAll(w, payload[start:end]); err != nil {
			return err
		}
		start = end
		if start < len(payload) && strategy.DelayBetweenFragments > 0 {
			time.Sleep(strategy.DelayBetweenFragments)
		}
	}
	return nil
}

func onlyDirect(techniques []Technique) bool {
	return len(techniques) == 1 && techniques[0] == TechniqueDirect
}

func normalizedSplitPoints(points []int, payloadLen int) []int {
	if len(points) == 0 {
		return nil
	}

	copyPoints := append([]int(nil), points...)
	sort.Ints(copyPoints)
	out := make([]int, 0, len(copyPoints))
	last := -1
	for _, p := range copyPoints {
		if p <= 0 || p >= payloadLen || p == last {
			continue
		}
		out = append(out, p)
		last = p
	}
	return out
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
