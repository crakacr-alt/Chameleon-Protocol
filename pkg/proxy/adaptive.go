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
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
)

var ErrUnsupportedCarrier = errors.New("carrier is not supported by TCP proxy")

type NetworkFunc func() (networkctx.Context, error)
type TCPDialFunc func(context.Context, string) (net.Conn, error)

// AdaptiveDialer executes plans produced by pkg/planner for SOCKS CONNECT.
type AdaptiveDialer struct {
	Planner       *planner.Planner
	Carriers      []carrier.Candidate
	DPIStrategies []dpi.Strategy
	PSK           string
	Purpose       string
	Timeout       time.Duration

	Network    NetworkFunc
	DirectDial TCPDialFunc
}

// DialContext chooses a plan, tries it, records hard route failures and
// automatically replans. A successful TCP dial is recorded as carrier success;
// DPI success is recorded only after real response bytes arrive.
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
	if a.Timeout <= 0 {
		a.Timeout = 8 * time.Second
	}
	if a.Network == nil {
		a.Network = networkctx.Detect
	}
	if a.DirectDial == nil {
		dialer := &net.Dialer{Timeout: a.Timeout}
		a.DirectDial = func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		}
	}

	network, err := a.Network()
	if err != nil {
		network = networkctx.Context{ID: "unknown", Link: networkctx.LinkUnknown}
	}
	if network.ID == "" {
		network.ID = "unknown"
	}

	req := planner.Request{
		Network:       network,
		Destination:   destination,
		Protocol:      "tcp",
		Purpose:       a.Purpose,
		Carriers:      a.Carriers,
		DPIStrategies: a.DPIStrategies,
	}

	maxAttempts := len(a.Carriers) + 2
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
		conn, err := a.dialPlan(ctx, destination, plan)
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

		return newLearningConn(conn, a.Planner, plan, destination, connectLatency), nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("adaptive plan exhausted without a usable TCP carrier")
	}
	return nil, lastErr
}

func (a *AdaptiveDialer) dialPlan(ctx context.Context, destination string, plan planner.Plan) (net.Conn, error) {
	strategy := plan.DPI.Strategy
	switch plan.Carrier.Carrier.Kind {
	case carrier.KindDirect:
		conn, err := a.DirectDial(ctx, destination)
		if err != nil {
			return nil, err
		}
		return dpi.NewFirstWriteConn(conn, strategy), nil

	case carrier.KindChameleonTCP:
		endpoint := plan.Carrier.Carrier.Endpoint
		if endpoint == "" {
			return nil, fmt.Errorf("chameleon TCP carrier has no endpoint")
		}
		return tunnel.DialContextWithDialer(ctx, endpoint, destination, a.PSK, a.Timeout,
			func(ctx context.Context, address string) (net.Conn, error) {
				conn, err := a.DirectDial(ctx, address)
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
		return socks5.DialContextWithDialer(ctx, endpoint, destination, a.Timeout,
			func(ctx context.Context, address string) (net.Conn, error) {
				conn, err := a.DirectDial(ctx, address)
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

type learningConn struct {
	net.Conn
	planner        *planner.Planner
	plan           planner.Plan
	destination    string
	connectLatency time.Duration
	started        time.Time

	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
	closeOnce    sync.Once
}

func newLearningConn(conn net.Conn, p *planner.Planner, plan planner.Plan, destination string, connectLatency time.Duration) net.Conn {
	return &learningConn{
		Conn:           conn,
		planner:        p,
		plan:           plan,
		destination:    destination,
		connectLatency: connectLatency,
		started:        time.Now(),
	}
}

func (c *learningConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.bytesRead.Add(int64(n))
	}
	return n, err
}

func (c *learningConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.bytesWritten.Add(int64(n))
	}
	return n, err
}

func (c *learningConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() {
		read := c.bytesRead.Load()
		if read <= 0 {
			return
		}
		elapsed := time.Since(c.started)
		seconds := elapsed.Seconds()
		if seconds < 0.001 {
			seconds = 0.001
		}
		throughput := float64(read+c.bytesWritten.Load()) / seconds
		_ = c.planner.Observe(planner.Result{
			Plan:        c.plan,
			Destination: c.destination,
			Protocol:    "tcp",
			Success:     true,
			Scope:       planner.ScopeDPI,
			Latency:     c.connectLatency,
			Throughput:  throughput,
		})
	})
	return err
}
