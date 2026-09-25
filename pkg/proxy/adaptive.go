package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/socks5"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/traffic"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
)

var ErrUnsupportedCarrier = errors.New("carrier is not supported by TCP proxy")

type NetworkFunc func() (networkctx.Context, error)
type TCPDialFunc func(context.Context, string) (net.Conn, error)

// AdaptiveDialer executes plans produced by pkg/planner for SOCKS CONNECT.
// The configuration is treated as immutable after construction so concurrent
// SOCKS sessions can safely call DialContext.
type AdaptiveDialer struct {
	Planner       *planner.Planner
	Carriers      []carrier.Candidate
	DPIStrategies []dpi.Strategy
	PSK           string
	Purpose       string
	Timeout       time.Duration

	// DirectCooldown is how long direct is temporarily skipped after every
	// available userspace DPI strategy recently failed for the same destination.
	DirectCooldown time.Duration

	// ApplicationFailureWindow limits automatic DPI-failure classification
	// to early failures after a TCP connection was already established.
	ApplicationFailureWindow time.Duration

	Network    NetworkFunc
	DirectDial TCPDialFunc
}

// DialContext chooses a plan, tries it, records hard route failures and
// automatically replans. A successful TCP dial is recorded as carrier success.
// For direct traffic, DPI success/failure is learned from real application
// bytes. For tunnel/relay carriers, their own handshake proves the first hop.
func (a *AdaptiveDialer) DialContext(ctx context.Context, destination string) (net.Conn, error) {
	if a == nil || a.Planner == nil {
		return nil, fmt.Errorf("adaptive dialer is not initialized")
	}
	if len(a.Carriers) == 0 {
		return nil, fmt.Errorf("no carriers configured")
	}
	if len(a.DPIStrategies) == 0 {
		return nil, fmt.Errorf("no DPI strategies configured")
	}

	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	directCooldown := a.DirectCooldown
	if directCooldown <= 0 {
		directCooldown = 10 * time.Minute
	}
	failureWindow := a.ApplicationFailureWindow
	if failureWindow <= 0 {
		failureWindow = 12 * time.Second
	}

	networkFunc := a.Network
	if networkFunc == nil {
		networkFunc = networkctx.Detect
	}

	directDial := a.DirectDial
	if directDial == nil {
		dialer := &net.Dialer{Timeout: timeout}
		directDial = func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		}
	}

	network, err := networkFunc()
	if err != nil {
		network = networkctx.Context{ID: "unknown", Link: networkctx.LinkUnknown}
	}
	if network.ID == "" {
		network.ID = "unknown"
	}

	class := traffic.Classify(traffic.Hint{
		Destination: destination,
		Protocol:    "tcp",
		Purpose:     a.Purpose,
	})

	candidates := append([]carrier.Candidate(nil), a.Carriers...)
	directDPIContext := dpi.Context{
		NetworkID:    network.ID,
		Destination:  destination,
		TrafficClass: string(class),
	}
	if len(candidates) > 1 &&
		a.Planner.DPI.RecentlyExhausted(directDPIContext, a.DPIStrategies, time.Now(), directCooldown) {
		candidates = withoutDirectCarrier(candidates)
	}

	req := planner.Request{
		Network:       network,
		Destination:   destination,
		Protocol:      "tcp",
		Purpose:       a.Purpose,
		Carriers:      candidates,
		DPIStrategies: a.DPIStrategies,
	}

	maxAttempts := len(candidates) + 2
	seen := make(map[string]bool)
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		plan, err := a.Planner.Choose(req)
		if err != nil {
			return nil, err
		}
		key := plan.Carrier.Carrier.Name + "|" + plan.DPI.Strategy.Name + "|" + plan.DPITarget
		if seen[key] {
			break
		}
		seen[key] = true

		started := time.Now()
		conn, err := a.dialPlan(ctx, destination, plan, timeout, directDial)
		connectLatency := time.Since(started)
		if err != nil {
			lastErr = err
			_ = a.Planner.Observe(planner.Result{
				Plan:        plan,
				Destination: destination,
				Protocol:    "tcp",
				Success:     false,
				Scope:       planner.ScopeCarrier,
				Latency:     connectLatency,
				Failure:     err.Error(),
			})
			continue
		}

		if err := a.Planner.Observe(planner.Result{
			Plan:        plan,
			Destination: destination,
			Protocol:    "tcp",
			Success:     true,
			Scope:       planner.ScopeCarrier,
			Latency:     connectLatency,
		}); err != nil {
			_ = conn.Close()
			return nil, err
		}

		learnApplicationDPI := plan.Carrier.Carrier.Kind == carrier.KindDirect
		if !learnApplicationDPI {
			// The tunnel/SOCKS handshake itself exchanged authenticated or
			// protocol-valid bytes with the visible first-hop endpoint.
			if err := a.Planner.Observe(planner.Result{
				Plan:        plan,
				Destination: destination,
				Protocol:    "tcp",
				Success:     true,
				Scope:       planner.ScopeDPI,
				Latency:     connectLatency,
			}); err != nil {
				_ = conn.Close()
				return nil, err
			}
		}

		return newLearningConn(
			conn,
			a.Planner,
			plan,
			destination,
			connectLatency,
			failureWindow,
			learnApplicationDPI,
		), nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("adaptive plan exhausted without a usable TCP carrier")
	}
	return nil, lastErr
}

func (a *AdaptiveDialer) dialPlan(
	ctx context.Context,
	destination string,
	plan planner.Plan,
	timeout time.Duration,
	directDial TCPDialFunc,
) (net.Conn, error) {
	strategy := plan.DPI.Strategy
	switch plan.Carrier.Carrier.Kind {
	case carrier.KindDirect:
		conn, err := directDial(ctx, destination)
		if err != nil {
			return nil, err
		}
		return dpi.NewFirstWriteConn(conn, strategy), nil

	case carrier.KindChameleonTCP:
		endpoint := plan.Carrier.Carrier.Endpoint
		if endpoint == "" {
			return nil, fmt.Errorf("chameleon TCP carrier has no endpoint")
		}
		return tunnel.DialContextWithDialer(ctx, endpoint, destination, a.PSK, timeout,
			func(ctx context.Context, address string) (net.Conn, error) {
				conn, err := directDial(ctx, address)
				if err != nil {
					return nil, err
				}
				return dpi.NewFirstWriteConn(conn, strategy), nil
			})

	case carrier.KindRelay:
		endpoint := plan.Carrier.Carrier.Endpoint
		if endpoint == "" {
			return nil, fmt.Errorf("relay carrier has no endpoint")
		}
		return socks5.DialContextWithDialer(ctx, endpoint, destination, timeout,
			func(ctx context.Context, address string) (net.Conn, error) {
				conn, err := directDial(ctx, address)
				if err != nil {
					return nil, err
				}
				return dpi.NewFirstWriteConn(conn, strategy), nil
			})

	case carrier.KindChameleonUDP:
		return nil, ErrUnsupportedCarrier
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedCarrier, plan.Carrier.Carrier.Kind)
	}
}

func withoutDirectCarrier(candidates []carrier.Candidate) []carrier.Candidate {
	out := make([]carrier.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Kind == carrier.KindDirect {
			continue
		}
		out = append(out, candidate)
	}
	if len(out) == 0 {
		return candidates
	}
	return out
}

type learningConn struct {
	net.Conn
	planner        *planner.Planner
	plan           planner.Plan
	destination    string
	connectLatency time.Duration
	failureWindow  time.Duration
	learnDPI       bool
	started        time.Time

	bytesRead      atomic.Int64
	bytesWritten   atomic.Int64
	firstWriteAt   atomic.Int64
	writeAttempted atomic.Bool
	deadlineOnce   sync.Once
	outcomeOnce    sync.Once
}

func newLearningConn(
	conn net.Conn,
	p *planner.Planner,
	plan planner.Plan,
	destination string,
	connectLatency time.Duration,
	failureWindow time.Duration,
	learnDPI bool,
) net.Conn {
	return &learningConn{
		Conn:           conn,
		planner:        p,
		plan:           plan,
		destination:    destination,
		connectLatency: connectLatency,
		failureWindow:  failureWindow,
		learnDPI:       learnDPI,
		started:        time.Now(),
	}
}

func (c *learningConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.bytesRead.Add(int64(n))
		if c.learnDPI {
			_ = c.Conn.SetReadDeadline(time.Time{})
		}
		c.observeDPISuccess()
	}
	if n == 0 && err != nil {
		c.maybeObserveDPIFailure(err)
	}
	return n, err
}

func (c *learningConn) Write(p []byte) (int, error) {
	if len(p) > 0 && c.learnDPI && c.writeAttempted.CompareAndSwap(false, true) {
		now := time.Now()
		c.firstWriteAt.Store(now.UnixNano())
		if c.expectsEarlyResponse() {
			c.deadlineOnce.Do(func() {
				_ = c.Conn.SetReadDeadline(now.Add(c.failureWindow))
			})
		}
	}

	n, err := c.Conn.Write(p)
	if n > 0 {
		c.bytesWritten.Add(int64(n))
	}
	if err != nil {
		c.maybeObserveDPIFailure(err)
	}
	return n, err
}

func (c *learningConn) Close() error {
	return c.Conn.Close()
}

func (c *learningConn) observeDPISuccess() {
	if !c.learnDPI {
		return
	}
	c.outcomeOnce.Do(func() {
		elapsed := time.Since(c.started)
		seconds := elapsed.Seconds()
		if seconds < 0.001 {
			seconds = 0.001
		}
		throughput := float64(c.bytesRead.Load()+c.bytesWritten.Load()) / seconds
		_ = c.planner.Observe(planner.Result{
			Plan:                   c.plan,
			Destination:            c.destination,
			Protocol:               "tcp",
			Success:                true,
			Scope:                  planner.ScopeDPI,
			CarrierAlreadyObserved: true,
			Latency:                c.connectLatency,
			Throughput:             throughput,
		})
	})
}

func (c *learningConn) maybeObserveDPIFailure(err error) {
	if !c.learnDPI {
		return
	}
	firstWrite := c.firstWriteAt.Load()
	if firstWrite == 0 {
		return
	}
	elapsed := time.Since(time.Unix(0, firstWrite))
	if !likelyDirectDPIFailure(
		c.plan,
		c.writeAttempted.Load(),
		c.bytesRead.Load(),
		err,
		elapsed,
		c.failureWindow,
	) {
		return
	}

	c.outcomeOnce.Do(func() {
		_ = c.planner.Observe(planner.Result{
			Plan:                   c.plan,
			Destination:            c.destination,
			Protocol:               "tcp",
			Success:                false,
			Scope:                  planner.ScopeDPI,
			CarrierAlreadyObserved: true,
			Latency:                elapsed,
			Failure:                err.Error(),
		})
	})
}

func (c *learningConn) expectsEarlyResponse() bool {
	return c.plan.TrafficClass == traffic.ClassWeb ||
		c.plan.TrafficClass == traffic.ClassStreaming
}
