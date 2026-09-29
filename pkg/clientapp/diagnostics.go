package clientapp

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/tunnel"
)

// TransportCheck is one real network check used by desktop and Android Doctor.
// It never contains the PSK or certificate private material.
type TransportCheck struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	OK        bool   `json:"ok"`
	Attempts  int    `json:"attempts"`
	Successes int    `json:"successes"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type diagnosticTask struct {
	name   string
	target string
	run    func(context.Context) error
}

// ProbeTransports performs real first-hop checks. TLS/QUIC checks include
// Chameleon authentication, so success proves more than an open port.
func ProbeTransports(ctx context.Context, cfg clientconfig.Config, samples int, timeout time.Duration) []TransportCheck {
	if samples <= 0 {
		samples = 1
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	tlsCfg := tunnel.TLSClientConfig{
		ServerName:   cfg.TLSServerName,
		PinnedSHA256: cfg.TLSFingerprint,
	}

	tasks := make([]diagnosticTask, 0, 4)
	if cfg.TLSServer != "" {
		target := cfg.TLSServer
		tasks = append(tasks, diagnosticTask{
			name:   "tcp-connect",
			target: target,
			run: func(checkCtx context.Context) error {
				dialer := &net.Dialer{Timeout: timeout}
				conn, err := dialer.DialContext(checkCtx, "tcp", target)
				if err != nil {
					return err
				}
				_ = conn.Close()
				return nil
			},
		})
		tasks = append(tasks, diagnosticTask{
			name:   "tls",
			target: target,
			run: func(checkCtx context.Context) error {
				_, err := tunnel.ProbeTLSContext(checkCtx, target, cfg.PSK, timeout, tlsCfg)
				return err
			},
		})
	}
	if cfg.QUICServer != "" {
		target := cfg.QUICServer
		tasks = append(tasks, diagnosticTask{
			name:   "quic",
			target: target,
			run: func(checkCtx context.Context) error {
				_, err := tunnel.ProbeQUICContext(checkCtx, target, cfg.PSK, timeout, tlsCfg)
				return err
			},
		})
	}
	if cfg.TCPServer != "" {
		target := cfg.TCPServer
		tasks = append(tasks, diagnosticTask{
			name:   "raw-tcp",
			target: target,
			run: func(checkCtx context.Context) error {
				_, err := tunnel.ProbeTCPContext(checkCtx, target, cfg.PSK, timeout)
				return err
			},
		})
	}

	results := make([]TransportCheck, len(tasks))
	var wg sync.WaitGroup
	for i := range tasks {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = measureTransport(ctx, tasks[i], samples)
		}()
	}
	wg.Wait()
	return results
}

func measureTransport(ctx context.Context, task diagnosticTask, samples int) TransportCheck {
	check := TransportCheck{
		Name:     task.name,
		Target:   task.target,
		Attempts: samples,
	}
	var total time.Duration
	var lastErr error

	for i := 0; i < samples; i++ {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		started := time.Now()
		err := task.run(ctx)
		elapsed := time.Since(started)
		if err != nil {
			lastErr = err
			continue
		}
		check.Successes++
		total += elapsed
	}

	check.OK = check.Successes > 0
	if check.Successes > 0 {
		check.LatencyMS = (total / time.Duration(check.Successes)).Milliseconds()
	}
	if !check.OK && lastErr != nil {
		check.Error = lastErr.Error()
	}
	return check
}
