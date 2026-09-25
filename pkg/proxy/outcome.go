package proxy

import (
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
)

// likelyDirectDPIFailure is deliberately conservative.
//
// A TCP connection was already established, so carrier reachability is known.
// We only classify a later failure as DPI/application-path evidence when:
//   - the carrier is direct;
//   - the client actually sent bytes;
//   - no response byte ever arrived;
//   - the stream ended quickly with a terminal network-style error.
//
// This is evidence, not proof. The adaptive learner is expected to compare
// multiple strategies rather than treating one observation as ground truth.
func likelyDirectDPIFailure(
	plan planner.Plan,
	bytesWritten int64,
	bytesRead int64,
	err error,
	elapsed time.Duration,
	window time.Duration,
) bool {
	if plan.Carrier.Carrier.Kind != carrier.KindDirect {
		return false
	}
	if bytesWritten <= 0 || bytesRead > 0 || err == nil {
		return false
	}
	if window <= 0 {
		window = 12 * time.Second
	}
	if elapsed < 0 || elapsed > window {
		return false
	}

	if errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
