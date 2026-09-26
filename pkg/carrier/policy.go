package carrier

// ScorePolicy scales the relative cost of route characteristics.
//
// Values are multipliers over the normal traffic-class scoring rules.
// A zero value is normalized to 1 so old callers preserve 1.0 behavior.
type ScorePolicy struct {
	LatencyScale    float64 `json:"latency_scale"`
	JitterScale     float64 `json:"jitter_scale"`
	ThroughputScale float64 `json:"throughput_scale"`
	CostScale       float64 `json:"cost_scale"`
}

func DefaultScorePolicy() ScorePolicy {
	return ScorePolicy{
		LatencyScale:    1,
		JitterScale:     1,
		ThroughputScale: 1,
		CostScale:       1,
	}
}

func (p ScorePolicy) normalized() ScorePolicy {
	if p.LatencyScale <= 0 {
		p.LatencyScale = 1
	}
	if p.JitterScale <= 0 {
		p.JitterScale = 1
	}
	if p.ThroughputScale <= 0 {
		p.ThroughputScale = 1
	}
	if p.CostScale <= 0 {
		p.CostScale = 1
	}
	return p
}

// SetScorePolicy changes scoring weights without discarding learned evidence.
func (e *Engine) SetScorePolicy(policy ScorePolicy) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policy = policy.normalized()
}

func (e *Engine) scorePolicyLocked() ScorePolicy {
	return e.policy.normalized()
}
