# Changelog

## [0.2.0-dev] - 2026-09-25

### Added
- context-aware path learner keyed by network, destination, traffic class, carrier and DPI strategy
- EWMA route metrics for latency, jitter, loss, throughput and overhead
- exploration bonus, hysteresis and repeated-failure escape logic
- adaptive DPI strategy engine with direct-first escalation and per-context memory
- protocol-scoped DPI policies inspired by ByeDPI/zapret concepts without embedding their code
- autopilot controller combining route and DPI learning into a bounded candidate plan
- architecture notes in `docs/adaptive_v0_2.md`

### Design
- Tailscale/DERP is treated as an optional carrier/fallback rather than the protocol foundation
- packet manipulation is kept behind future platform adapters so the control plane stays portable
- no cloud ML or paid control plane is required for learning

# Changelog

## [0.1.0] - 2026-07-16

### Added
- adaptive learner persistence via JSON
- deterministic epoch-based profile rotation
- traffic normalization with padding and jitter
- AEAD payload encryption via AES-GCM
- minimal X25519 key-exchange primitive for research sessions
- transport lifecycle session state tracking
- release-ready project README and publication guidance

### Changed
- transport send path hardened to avoid holding the write mutex during jitter sleep
- profile resolution now preserves configured transport profile

### Notes
- protocol remains a research prototype and is not a full production-ready adversarial transport system
