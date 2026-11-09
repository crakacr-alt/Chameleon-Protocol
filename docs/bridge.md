# Bridge — внешний маршрут

Bridge is a generic Android mode for running Chameleon as a local SOCKS5 proxy
**alongside an already active VPN or other working network route**. It does not
start Android `VpnService`, so it does not disconnect an existing VPN.

The mode is not tied to Tailscale. Examples include a reachable private overlay
address, an IPv6 endpoint, or a directly reachable server endpoint.

## How to use on Android

1. Establish an external route first (if required), e.g. connect to your tailnet.
2. Import the **existing Chameleon Auth v2** profile. Never publish it: it
   contains secrets. Do not replace the server's PSK/client credentials.
3. Choose **Bridge — внешний маршрут (SOCKS5)**.
4. Tap **Bridge: адрес сервера** and enter the reachable first-hop server
   endpoint `host:port`. For IPv6, use `[IPv6-address]:port`.
   Leave empty to use the address in the imported profile.
5. Tap connect. Bridge checks the TLS/TCP listener and Chameleon authentication
   over the **currently active Android route** before reporting success.
6. Configure a proxy-aware application to use `SOCKS5 127.0.0.1:1080`.

Example: if the VPS is reachable over a private overlay at
`100.101.102.103` and Chameleon listens on TLS `9443`, enter
`100.101.102.103:9443`. This address is only an example.

If using Tailscale on Android, the Chameleon app must **not** be excluded from
Tailscale's app-based split tunneling. The VPS must also be a tailnet peer,
accessible over the overlay IP, and listening on the selected port.

## Security / technical behavior

- Bridge forces Chameleon proxy mode (no direct Internet fallback).
- It uses authenticated Chameleon TLS/TCP with existing client credentials.
- Original TLS certificate pin is retained when using an overlay address.
- Bridge uses TCP only; QUIC/UDP are disabled. DNS over SOCKS5 UDP can fall
  back to the encrypted DNS-over-TCP path in Chameleon proxy mode.
- Bridge is local SOCKS5, not system-wide VPN. Other applications do not
  automatically use it. Tailscale's active system VPN remains in charge.
- The Bridge first-hop address is saved only in private app preferences.
  The imported profile's original address and secrets are unchanged.

## Limitations

- If no route can reach any Chameleon endpoint, neither Bridge nor changes to
  TLS framing can make a fully blocked IP reachable.
- A private WARP `wg0` IPv6 address is not necessarily a publicly accessible
  inbound IPv6 endpoint; verify provider IPv6 and inbound routing separately.
- While Android permits only one active `VpnService` per profile, a local
  proxy can coexist with that VPN.
- Bridge does not guarantee continuity when the external VPN itself disconnects.
- Do not enter a public IP that is still blocked unless the external VPN
  provides a working full-tunnel route to that public IP.

## VPS checks

On the VPS, check the address and port without revealing your profile secrets:

```bash
tailscale ip -4
ss -lntup | grep -E ':(443|9443)\\b'
ip -6 -br addr show scope global
```

The Tailscale command works only if Tailscale is installed and the VPS is a
member of your tailnet.

## Testing

- In Bridge diagnostics, verify successful authenticated TLS/TCP probe to the
  configured first-hop endpoint.
- Confirm Tailscale stays connected after Bridge is started.
- From an app that supports SOCKS5, verify traffic via `127.0.0.1:1080`.
- Stop external VPN: Bridge should fail its next attempt rather than silently
  use direct for proxy traffic.

Bridge is a universal generic mode, not a VPN chaining mechanism.
