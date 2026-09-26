# Chameleon first-hop probing

Version 0.9.2 adds active measurements for the path between a client and the
Chameleon server.

The probe is intentionally authenticated. A successful sample means:

1. the transport connected;
2. TLS/QUIC validation passed where applicable;
3. the Chameleon PSK handshake completed;
4. the server returned the encrypted probe response.

This is stronger evidence than "port is open", but it still does not mean every
final Internet destination is reachable.

## CLI

```bash
export CHAMELEON_TUNNEL_PSK='...'

chameleon-probe \
  --server=example.com:443 \
  --tls-fingerprint=SHA256_PIN \
  --transports=quic,tls,tcp \
  --samples=3
```

Source-tree equivalent:

```bash
go run ./cmd/probe ...
```

Example output:

```text
Chameleon 0.9.2 first-hop probe
server: example.com:443
resolved: ipv6=[2001:db8::1]:443 ipv4=203.0.113.10:443
quic  success=3/3 loss=0% avg=42ms jitter=4ms
tls   success=3/3 loss=0% avg=55ms jitter=7ms
tcp   success=3/3 loss=0% avg=38ms jitter=2ms
```

For scripts and future GUI/doctor integration use:

```bash
chameleon-probe ... --json
```

## IPv4 / IPv6 racing

`pkg/probe` resolves at most one useful IPv6 and one useful IPv4 candidate and
can race them with a small stagger.

The point is not to always prefer IPv6. The point is to avoid a long timeout
when IPv6 exists in DNS but is broken on the current access network.

No application payload is sent before the winning connection is returned.

## Metrics

The common summary contains:

- attempts;
- successes;
- failures;
- average latency;
- inter-sample jitter;
- loss ratio.

These values are transport-neutral so later client policy can compare TCP, TLS
and QUIC without introducing protocol-specific scoring formats.

## Relationship with adaptive memory

Active probes do not replace real traffic learning.

Real sessions remain the strongest evidence because they include destination
reachability and application response behavior.

Probes are useful for:

- a newly seen network;
- recovery after stale failures;
- diagnosing Wi-Fi vs mobile differences;
- selecting a likely first-hop fallback before user traffic waits for a long timeout.

## Privacy

The probe target is the configured Chameleon server. The dedicated probe
session does not ask the server to connect to an external destination.

No browsing destination is required for first-hop health measurement.
