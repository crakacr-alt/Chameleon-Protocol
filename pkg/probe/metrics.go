package probe

import (
	"context"
	"math"
	"time"
)

// Sample is one active first-hop measurement.
type Sample struct {
	Latency time.Duration
	Err     error
}

// Summary is deliberately transport-neutral. Carrier learning can consume the
// same metrics regardless of whether they came from TCP, TLS or QUIC.
type Summary struct {
	Attempts   int
	Successes  int
	Failures   int
	AvgLatency time.Duration
	Jitter     time.Duration
	LossRate   float64
}

// Measure runs an active probe several times. The caller controls the actual
// protocol handshake, so this package never substitutes ICMP reachability for
// application-path health.
func Measure(
	ctx context.Context,
	samples int,
	interval time.Duration,
	probe func(context.Context) error,
) Summary {
	if samples <= 0 {
		samples = 1
	}
	results := make([]Sample, 0, samples)
	for i := 0; i < samples; i++ {
		if err := ctx.Err(); err != nil {
			results = append(results, Sample{Err: err})
			break
		}

		started := time.Now()
		err := probe(ctx)
		results = append(results, Sample{
			Latency: time.Since(started),
			Err:     err,
		})

		if i+1 < samples && interval > 0 {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return Summarize(results)
			case <-timer.C:
			}
		}
	}
	return Summarize(results)
}

// Summarize calculates latency, inter-sample jitter and failure ratio.
func Summarize(samples []Sample) Summary {
	summary := Summary{Attempts: len(samples)}
	var (
		totalLatency time.Duration
		lastLatency  time.Duration
		jitterTotal  time.Duration
		jitterCount  int
	)
	for _, sample := range samples {
		if sample.Err != nil {
			summary.Failures++
			continue
		}
		summary.Successes++
		totalLatency += sample.Latency
		if lastLatency > 0 {
			delta := sample.Latency - lastLatency
			if delta < 0 {
				delta = -delta
			}
			jitterTotal += delta
			jitterCount++
		}
		lastLatency = sample.Latency
	}
	if summary.Successes > 0 {
		summary.AvgLatency = totalLatency / time.Duration(summary.Successes)
	}
	if jitterCount > 0 {
		summary.Jitter = jitterTotal / time.Duration(jitterCount)
	}
	if summary.Attempts > 0 {
		summary.LossRate = float64(summary.Failures) / float64(summary.Attempts)
	}
	// Avoid negative zero if callers serialize the value.
	if math.Abs(summary.LossRate) == 0 {
		summary.LossRate = 0
	}
	return summary
}
