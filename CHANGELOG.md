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
