# Chameleon Protocol

Chameleon Protocol is a research-oriented adaptive transport wrapper for packet-flow normalization. The project explores how to shape traffic so that it looks like ordinary, profile-consistent activity while preserving low-overhead transport primitives for experiments and protocol resilience testing.

## Highlights

- traffic morphing across profile templates: WebRTC, HTTP/3, Gaming
- randomized packet normalization through padding
- bounded jitter scheduling for timing control
- deterministic epoch-based profile rotation
- AEAD protection for payloads using AES-GCM
- lightweight adaptive learner with persisted decisions
- reproducible benchmark scenario and metrics report

## Project goals

The protocol is designed for controlled research around:

1. flow normalization for traffic shaping
2. adaptive profile selection under changing network conditions
3. stealthy transport pattern imitation for evaluation of traffic classifiers
4. reproducible benchmark experiments over local UDP transport

## Current architecture

```text
chameleon-protocol/
├── cmd/
│   ├── client/      # client entrypoint
│   ├── server/      # server entrypoint
│   └── benchmark/   # benchmark entrypoint
├── pkg/
│   ├── adaptive/    # learning and persisted route decisions
│   ├── core/        # transport wrapper and packet framing
│   ├── crypto/      # AEAD and key-exchange primitives
│   ├── experiment/  # benchmark scenarios and metrics
│   ├── morph/       # padding and jitter shaping
│   └── state/       # deterministic epoch sync and rotation
└── go.mod
```

## Core components

### pkg/core

- Transport: UDP wrapper for normalization and sending
- Normalizer: randomized target-size shaping before wire emission
- BehaviorProfile: traffic-profile selection surface
- EncodeFrame / DecodeFrame: frame packing and validation

### pkg/morph

- PaddingConfig:
  creates randomized packet-length normalization windows
- JitterConfig:
  injects bounded delay to emulate realistic burst timing

### pkg/crypto

- Cipher:
  symmetric AEAD payload wrapper using AES-GCM
- KeyExchange:
  minimal X25519-based shared secret derivation for experiments

### pkg/state

- Sync:
  deterministic shared-secret epoch profile mapping
- EpochState:
  strict state-machine style epoch controller for profile rotation

### pkg/adaptive

- Learner:
  lightweight scoring memory for profile selection and persistence to JSON

### pkg/experiment

- Scenario:
  reproducible benchmark runner over loopback UDP
- Metrics:
  reportable throughput, latency, loss, and entropy stats

## Supported behavior profiles

- webrtc
- http3
- gaming

Each profile carries its own default padding and jitter windows.

## Security note

This project is an academic prototype and should not be treated as a full production transport. The current design uses:

- AEAD payload encryption for confidentiality
- deterministic epoch rotation for coordinated profile switching
- lightweight adaptive heuristics for route learning

This reduces direct traffic signatures, but it is still vulnerable to advanced statistical analysis if an attacker observes many samples. A more robust architecture would require:

- authenticated key exchange with session binding
- explicit handshake / rekey / epoch transition state machine
- per-epoch entropy budgeting and stronger anti-classification controls
- measured adversarial evaluation against traffic classifiers

## Quick start

### Requirements

- Go 1.22+
- Linux, macOS, or Windows with standard Go toolchain

### Server

```bash
go run ./cmd/server --address=127.0.0.1:9000 --psk=research-secret
```

### Client

```bash
go run ./cmd/client --target=127.0.0.1:9000 --profile=webrtc --burst=3 --psk=research-secret
```

### Benchmark

```bash
go run ./cmd/benchmark --profile=webrtc --burst=2 --rounds=1 --payload=hello-chameleon --psk=research-secret
```

### Run the full verification suite

```bash
go test ./...
```

## Release checklist

Before publishing the repository:

1. ensure the code is formatted with `gofmt`
2. run `go test ./...`
3. verify module metadata in `go.mod`
4. review the license and ownership details
5. publish a tagged release once the repository remote is configured

## GitHub publication commands

The workspace currently does not expose a Git repository root, so the exact publish step cannot be completed from this environment. The following commands are the correct release sequence to use once the repository is initialized locally or on GitHub:

```bash
git init

git add .

git commit -m "Initial release: Chameleon Protocol"

git branch -M main

git remote add origin https://github.com/<YOUR-USER>/<YOUR-REPO>.git

git push -u origin main
```

Optional tag for a versioned release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Status

The current repository is a working research prototype with stable test coverage and a clear modular layout for continued protocol research.

Further work should focus on:

- full session handshake and authenticated key lifecycle
- richer state-machine transitions for epoch rotation
- multi-scenario experimental reporting and profile comparison
- stronger anti-classification shaping budget control
