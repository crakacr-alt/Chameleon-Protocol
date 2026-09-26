package probe

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type closeTrackingConn struct {
	net.Conn
	closed chan struct{}
}

func (c *closeTrackingConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	if c.Conn != nil {
		return c.Conn.Close()
	}
	return nil
}

func TestRaceConnectionsUsesFastFirstPath(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	winner, failures, err := RaceConnections(context.Background(), []ConnAttempt{
		{
			Name: "direct",
			Dial: func(context.Context) (net.Conn, error) {
				return client, nil
			},
		},
		{
			Name:  "quic",
			Delay: 200 * time.Millisecond,
			Dial: func(context.Context) (net.Conn, error) {
				t.Fatal("fallback should not start when direct wins immediately")
				return nil, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer winner.Conn.Close()

	if winner.Name != "direct" {
		t.Fatalf("want direct winner, got %q", winner.Name)
	}
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %+v", failures)
	}
}

func TestRaceConnectionsFallsBackWhenDirectIsSlow(t *testing.T) {
	fallbackClient, fallbackServer := net.Pipe()
	defer fallbackServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	winner, _, err := RaceConnections(ctx, []ConnAttempt{
		{
			Name: "direct",
			Dial: func(ctx context.Context) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
		{
			Name:  "quic",
			Delay: 20 * time.Millisecond,
			Dial: func(context.Context) (net.Conn, error) {
				return fallbackClient, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer winner.Conn.Close()

	if winner.Name != "quic" {
		t.Fatalf("want quic winner, got %q", winner.Name)
	}
}

func TestRaceConnectionsReturnsFailures(t *testing.T) {
	_, failures, err := RaceConnections(context.Background(), []ConnAttempt{
		{
			Name: "direct",
			Dial: func(context.Context) (net.Conn, error) {
				return nil, errors.New("direct blocked")
			},
		},
		{
			Name: "quic",
			Dial: func(context.Context) (net.Conn, error) {
				return nil, errors.New("udp blocked")
			},
		},
	})
	if err == nil {
		t.Fatal("expected race failure")
	}
	if len(failures) != 2 {
		t.Fatalf("want 2 failures, got %d", len(failures))
	}
}
