# Android connection/auth reliability checklist

Release target: Android 1.0.11-alpha.

## Server path

- [x] Verify both VPS are online through Remote Desktop Commander.
- [x] Verify WireGuard handshake between ingress and exit.
- [x] Verify ingress `80.86.216.169` forwards TCP/443 and TCP/9443 through HAProxy.
- [x] Verify exit `88.210.20.127` runs Chameleon TLS on 9443.
- [x] Verify the real relay PSK and TLS fingerprint by authenticated tunnel test.
- [x] Verify TLS-only `80.86.216.169:443` reaches the exit and returns public IP `88.210.20.127`.

## Profile correctness

- [x] Stop assuming that a TLS listener is also a raw Chameleon TCP listener.
- [x] Stop inventing missing transports when a profile explicitly declares transport endpoints.
- [x] Keep legacy `CHAMELEON_SERVER` compatibility for old profiles.
- [x] Generate new server/relay profiles with explicit TLS preference.
- [x] Use `UDP_MODE=auto` so unavailable QUIC falls back to DNS-over-TCP without direct DNS leakage in Proxy/VPN mode.

## Android connection flow

- [x] Use full-device VPN as the first-run default on Android 10+.
- [x] Run authenticated preflight immediately after profile import.
- [x] Persist the endpoint/transport that actually passed authentication.
- [x] Show a clear success result after profile verification.
- [x] Keep a visible error when VPN startup fails.
- [x] Do not repeatedly retry a QUIC endpoint that already failed mobile preflight.

## Regression safety

- [x] Add regression test for explicit TLS-only profiles.
- [x] Keep legacy profile TCP fallback test.
- [x] Run full `go test ./...` on Go 1.27.
- [x] Run shell syntax checks for deployment scripts.
- [ ] GitHub Android build must pass before merge.
- [ ] Signed APK SHA-256 must be written back by release workflow after merge.

## Scope note

This release keeps the native Chameleon tunnel because the authenticated TLS relay path is proven working. It does not embed Xray into the APK; Xray/Happ-style protocol support can remain an optional additional backend rather than replacing a working native transport.
