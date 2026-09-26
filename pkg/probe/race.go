package probe

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ConnAttempt is one candidate connection attempt.
// Delay allows a Happy-Eyeballs style stagger: the cheap path starts first,
// while the next path starts only if the first one has not won quickly.
type ConnAttempt struct {
	Name  string
	Delay time.Duration
	Dial  func(context.Context) (net.Conn, error)
}

// ConnResult contains the measured connect result.
type ConnResult struct {
	Name    string
	Conn    net.Conn
	Latency time.Duration
	Err     error
}

// RaceConnections returns the first successful connection.
// Losing successful connections are closed, so probing doesn't leak sockets.
func RaceConnections(ctx context.Context, attempts []ConnAttempt) (ConnResult, []ConnResult, error) {
	if len(attempts) == 0 {
		return ConnResult{}, nil, fmt.Errorf("no connection attempts")
	}
	for _, attempt := range attempts {
		if attempt.Name == "" || attempt.Dial == nil {
			return ConnResult{}, nil, fmt.Errorf("invalid connection attempt")
		}
	}

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan ConnResult, len(attempts))
	var wg sync.WaitGroup
	var winner atomic.Bool

	for _, attempt := range attempts {
		attempt := attempt
		wg.Add(1)
		go func() {
			defer wg.Done()

			if attempt.Delay > 0 {
				timer := time.NewTimer(attempt.Delay)
				select {
				case <-raceCtx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					return
				case <-timer.C:
				}
			}

			started := time.Now()
			conn, err := attempt.Dial(raceCtx)
			result := ConnResult{
				Name:    attempt.Name,
				Conn:    conn,
				Latency: time.Since(started),
				Err:     err,
			}

			if err == nil && conn != nil {
				if winner.CompareAndSwap(false, true) {
					results <- result
					cancel()
					return
				}
				_ = conn.Close()
				return
			}

			select {
			case results <- result:
			case <-raceCtx.Done():
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	failures := make([]ConnResult, 0, len(attempts))
	for {
		select {
		case result := <-results:
			if result.Err == nil && result.Conn != nil {
				return result, failures, nil
			}
			failures = append(failures, result)
		case <-done:
			if len(failures) == 0 && ctx.Err() != nil {
				return ConnResult{}, failures, ctx.Err()
			}
			return ConnResult{}, failures, fmt.Errorf("all connection probes failed")
		case <-ctx.Done():
			return ConnResult{}, failures, ctx.Err()
		}
	}
}
