# Chameleon Roadmap

This roadmap is an engineering contract, not a marketing checklist. A version is
released only when its acceptance criteria are met.

## 0.9.0 — UDP / QUIC foundation

Goal: make the adaptive engine useful for real UDP traffic, not only TCP/SOCKS.

Planned:
- general-purpose UDP tunnel frames;
- UDP association/session table;
- SOCKS5 UDP ASSOCIATE;
- QUIC-capable carrier abstraction;
- RTT/jitter/loss probe model;
- IPv4/IPv6 candidate racing;
- PMTU-aware payload sizing;
- DNS UDP path;
- carrier race helper;
- diagnostics for active path and measurements.

Acceptance:
- UDP echo end-to-end through Chameleon;
- SOCKS5 UDP test;
- no TCP regression;
- race tests green;
- clean fallback when UDP carrier is unavailable.

## 1.0.0 — Stable automatic client

Goal: install, connect and use without understanding Chameleon internals.

Planned:
- Linux daemon/service client;
- Windows service client baseline;
- Android client core suitable for VpnService integration;
- Smart/Proxy/Full-Tunnel policy model;
- LAN/private bypass;
- destination policies;
- automatic network-change recovery;
- expiring adaptive memory;
- confidence/time-decay;
- `chameleon doctor`;
- diagnostic export without secrets;
- stable config schema;
- server/client compatibility negotiation.

Acceptance:
- one-command Linux server install;
- one-command Linux client install;
- automatic reconnect after network change;
- config upgrade test;
- no secret leakage in diagnostics;
- documented rollback to previous release.

## 1.1.0 — Desktop UX

Goal: make Chameleon understandable without CLI knowledge.

Planned:
- desktop control API;
- Windows/Linux GUI;
- Auto/Fast/Stable/Private/Gaming/Streaming modes;
- live carrier/DPI/latency status;
- connect/disconnect;
- per-site policy editor;
- safe log viewer/export.

## 1.2.0 — Android

Goal: first-class Android operation.

Planned:
- VpnService adapter;
- per-app routing;
- battery-aware probing;
- Wi-Fi/mobile context handling;
- QR bootstrap/import;
- foreground-service lifecycle;
- proxy/embedded coexistence mode.

## 1.3.0 — Router / gateway mode

Goal: devices can benefit without installing a Chameleon client.

Planned:
- OpenWrt/Linux gateway package;
- DNS forwarding;
- transparent TCP/UDP redirection adapters;
- LAN device policy;
- safe local-network bypass.

## 1.4.0 — Multi-ingress identity

Goal: server identity is independent from a single IP.

Planned:
- endpoint pool;
- IPv4/IPv6/port/TLS/QUIC ingress records;
- endpoint health and cooldown;
- signed server identity metadata;
- discovery cache;
- rotation without changing logical server identity.

## 1.5.0 — Trusted peer relay

Goal: recover when direct server ingress is unreachable.

Planned:
- user-owned trusted relay;
- relay authorization;
- no public exit relay by default;
- relay health scoring;
- loop prevention;
- bounded relay chains.

## 1.6.0 — Platform DPI backends

Goal: extend beyond userspace split where the operating system permits it.

Planned:
- common packet-control capability interface;
- Linux packet backend;
- Windows packet backend;
- router backend;
- strategy capability negotiation;
- conservative escalation from direct/userspace methods.

## 2.0.0 — Adaptive mesh sessions

Goal: logical sessions survive changes in network and carrier.

Planned:
- logical session IDs independent from TCP connection;
- resumable streams;
- multipath candidate set;
- Wi-Fi↔LTE continuation;
- IPv4↔IPv6 migration;
- ingress/relay migration;
- replay-safe resumption;
- bounded buffering and backpressure.

## Release discipline

Each completed version gets:
- CHANGELOG entry;
- VERSION update;
- frozen `versions/vX.Y.Z` branch;
- CI evidence;
- documentation updates;
- migration notes;
- no claims beyond tested behavior.
