# Chameleon v0.2 — Adaptive Path + DPI Autopilot

## Goal

Chameleon v0.2 turns the protocol from a fixed traffic-shaping prototype into a small self-learning control plane.

The key idea is simple:

1. try the cheapest normal path first;
2. measure what actually happened;
3. escalate only when the connection shows a real failure symptom;
4. remember the best carrier + strategy for this network and destination;
5. keep probing cheap and avoid route flapping.

No neural network, cloud API or paid control plane is required.

## Why not embed zapret or ByeDPI wholesale?

The useful ideas are architectural:

- automatic fallback after timeout/reset/TLS failure;
- caching the working strategy;
- multiple strategies for different protocols;
- applying workarounds only where needed;
- keeping the default path fast and unmodified.

Chameleon implements those ideas independently in Go. This avoids coupling the core to one OS, one packet interception framework or another project's release cycle.

## New packages

### pkg/adaptive/path_learning.go

Stores route experience using this context:

network + destination + traffic class + carrier + endpoint + DPI strategy

Metrics:

- success/failure;
- consecutive failures;
- EWMA latency;
- EWMA jitter;
- EWMA loss;
- EWMA throughput;
- EWMA overhead.

Traffic classes currently are:

- generic;
- web;
- interactive;
- streaming;
- bulk.

A candidate gets an exploration bonus when it has not been tried before. A working route is protected by hysteresis and is not replaced for tiny score differences.

### pkg/dpi/policy.go

The DPI engine is a policy layer, not a packet interceptor.

It currently knows these strategy names:

- direct;
- stream-split;
- tls-record-split;
- disorder;
- fake-packet;
- udp-shape.

The actual packet-level implementation belongs in platform adapters. This separation is intentional: Linux may use a different mechanism from Windows, Android or a router.

Failure hints currently include:

- timeout;
- reset;
- TLS handshake failure;
- redirect;
- UDP blackhole;
- high loss;
- incompatibility.

The engine keeps memory per network + destination + protocol and favors the cheapest strategy that has worked before.

### pkg/autopilot/controller.go

Autopilot combines route learning and DPI strategy learning.

It creates a short candidate plan instead of brute-forcing every possible combination. By default it crosses carriers with only the top three applicable DPI strategies.

After a connection attempt, one observation updates both learners.

## Intended carrier architecture

The current package stores carrier names as strings so the data model is not tied to a specific implementation.

Planned carrier families:

- direct UDP / QUIC;
- TCP + TLS;
- HTTP/2 or other ordinary web-compatible carrier;
- relay;
- Tailscale / DERP adapter;
- peer-assisted relay;
- additional platform-specific transports.

DERP is therefore a fallback carrier, not the foundation of the protocol.

## Client coexistence

The control plane owns no TUN/VPN interface.

This is deliberate. Future frontends can expose:

- proxy mode: local SOCKS/HTTP, coexists with an existing VPN;
- TUN mode: Chameleon owns the system tunnel;
- embedded mode: library integration into another application;
- gateway mode: Chameleon runs on a router or network gateway so endpoint devices require no installation.

On Android, two independent VPNService-based full-device VPNs cannot reliably coexist. Proxy/embedded/gateway modes are the preferred coexistence path.

## Important physical limit

If every packet to a server IP is dropped before it reaches the server, server software cannot repair that path by itself.

Resilience to endpoint-IP blocking requires at least one additional reachable ingress such as:

- another address already owned by the deployment;
- IPv6;
- relay;
- peer;
- another existing transport endpoint.

Chameleon can learn and select among reachable alternatives, but cannot create a new routable IP address from software alone.

## Performance policy

The fast path must remain fast.

Therefore v0.2 follows these rules:

- direct is preferred until evidence says otherwise;
- expensive DPI strategies carry a score cost;
- probe plans are bounded;
- EWMA avoids reacting to one noisy sample;
- hysteresis avoids constant switching;
- repeated hard failures override hysteresis;
- learning state is tiny JSON and updates are O(1).

## Next implementation steps

1. add active probe interface and cheap background measurements;
2. add carrier interface and direct UDP/QUIC + TCP/TLS implementations;
3. implement Linux DPI adapter first;
4. add destination/direct breakout policy;
5. add IPv4/IPv6 and PMTU diagnostics;
6. add session-resume layer above carriers;
7. add proxy frontend, then TUN and Android/gateway integrations;
8. add netem scenarios for UDP blocking, IP path failure, MTU blackholes and Wi-Fi/LTE transitions.
