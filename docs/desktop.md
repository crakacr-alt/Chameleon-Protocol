# Chameleon Desktop — 1.1

Chameleon 1.1 adds a localhost control API and a small built-in desktop panel.

The panel is intentionally embedded into the normal client process:

- no second routing engine;
- no duplicated adaptive state;
- no external JavaScript/CDN dependencies;
- the API binds only to loopback;
- the API never returns the tunnel PSK.

## Start

Normal client start:

```bash
chameleon connect
```

The SOCKS client remains on:

```text
127.0.0.1:1080
```

The desktop panel is available on:

```text
http://127.0.0.1:8765/
```

A custom loopback address may be selected:

```bash
chameleon connect --control=127.0.0.1:9876
```

A non-loopback bind is rejected.

## Presets

The desktop panel supports:

- `auto`;
- `fast`;
- `stable`;
- `gaming`;
- `streaming`.

These are not cosmetic modes.

They change the real scoring multipliers used by the existing Carrier Engine.

### Auto

Balanced baseline. All score multipliers are 1.

### Fast

Increases the weight of latency and slightly prefers cheaper paths.

### Stable

Raises the jitter penalty and accepts a little more route cost to avoid unstable
paths.

### Gaming

Strongly prioritizes latency and jitter over raw throughput.

### Streaming

Strongly raises throughput weight while reducing sensitivity to latency.

Changing a preset does not erase learned carrier observations. It changes how
the same measured evidence is interpreted.

The selected preset is saved to the normal client JSON config.

## Local API

### GET /api/status

Returns non-secret runtime information:

- version;
- client mode;
- selected preset;
- SOCKS address;
- network context;
- available QUIC/TLS/TCP carriers;
- UDP mode;
- number of bypass rules.

### GET /api/presets

Returns the supported preset names.

### POST /api/preset

Request:

```json
{"preset":"gaming"}
```

The runtime updates immediately and persists the new preset.

## Security boundary

The control service:

- listens only on loopback;
- rejects requests whose remote address is not loopback;
- never serializes the PSK;
- uses a small request body limit for mutation endpoints.

1.1 is a local desktop control surface, not a remotely administered server API.

## Why a built-in web panel first

A native Windows/Linux GUI can later wrap the same local API without moving
network policy into the UI.

This keeps:

```text
CLI
Desktop UI
future tray app
```

all attached to the same client daemon and adaptive engine.
