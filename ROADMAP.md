# Chameleon Protocol Roadmap

Roadmap описывает направление, а не обещание считать незавершённую функцию готовой.
Каждый пункт становится release только после tests + CI + documentation.

## 0.9.x — UDP foundation

### 0.9.0 — QUIC carrier
- real QUIC over UDP for reliable proxy streams;
- adaptive probing/racing;
- stale-evidence decay;
- TCP+UDP server deployment.

### 0.9.1 — UDP datagram path
- QUIC DATAGRAM data plane;
- SOCKS5 UDP ASSOCIATE;
- DNS/UDP use case;
- packet-size/MTU limits and tests.

### 0.9.2 — network probing
- IPv4/IPv6 racing;
- explicit QUIC/TLS first-hop probes;
- RTT/jitter/loss observations;
- better automatic failure classification.

## 1.0.0 — First user release

Status: implemented in release/v1.0.0; release requires green CI.
- one unified client command;
- config/profile import;
- Smart / Proxy modes;
- Linux daemon installer;
- automatic reconnect;
- diagnostics/doctor;
- stable config migration contract;
- documented Windows binary/service workflow.

## 1.1 — Desktop UX

Status: implemented in release/v1.1.0; release requires green CI.
- local control API;
- desktop status UI;
- Auto/Fast/Stable/Gaming/Streaming policy presets;
- live carrier/network diagnostics.

## 1.2 — Android
- Android VpnService client;
- per-app routing;
- QR/profile import;
- battery-aware probing;
- proxy/embedded mode for coexistence scenarios.

## 1.3 — Router/Gateway
- OpenWrt/Linux gateway mode;
- DNS integration;
- LAN bypass;
- device policy groups.

## 1.4 — Multi-ingress
- server identity independent from one IP;
- endpoint pool;
- IPv4/IPv6/alternate-port discovery;
- signed endpoint profile.

## 1.5 — Peer relay
- trusted private relay/bridge;
- no public exit relay by default;
- explicit authorization and loop prevention.

## 1.6 — Platform DPI backends
- capability API;
- Linux packet-control backend;
- Windows packet interception backend;
- router backend;
- only activate expensive techniques when measured evidence requires them.

## 2.0 — Adaptive Mesh
- multiple server/relay paths;
- logical session identity across carrier changes;
- path handoff;
- multi-endpoint health and routing;
- compatibility/migration rules for 1.x clients.

## Release rule

A feature is not marked Ready until there is an executor/data path behind it.
Architecture placeholders are labeled experimental or planned.
