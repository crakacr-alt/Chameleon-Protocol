# UDP data path — Chameleon 0.9.1

0.9.1 добавляет настоящий application UDP path поверх QUIC DATAGRAM.

## Поток

```text
Application
   ↓ SOCKS5 UDP ASSOCIATE
127.0.0.1 local UDP relay
   ↓
Chameleon UDP association
   ├── direct UDP
   └── QUIC DATAGRAM → Chameleon VPS → UDP destination
```

SOCKS5 и tunnel packages связаны интерфейсом, а не прямой зависимостью:

```go
type UDPAssociation interface {
    Send(context.Context, string, []byte) error
    Receive(context.Context) (string, []byte, error)
    Close() error
}
```

Это позволит последующим версиям подключать router, Android и другие UDP backend без переписывания SOCKS5.

## Режимы клиента

`--udp-mode=auto` — по умолчанию. Если настроен `--chameleon-quic`, UDP идёт через QUIC DATAGRAM; без QUIC endpoint используется direct UDP.

`--udp-mode=direct` — всегда direct UDP.

`--udp-mode=quic` — всегда Chameleon QUIC; без `--chameleon-quic` считается ошибкой конфигурации.

## Границы пакетов

UDP packet boundary сохраняется от SOCKS5 datagram до QUIC DATAGRAM и обратно.

SOCKS5 FRAG != 0 намеренно отклоняется. Chameleon не притворяется, что умеет безопасно собирать неизвестную application fragmentation.

## Размер

0.9.1 использует консервативный Chameleon datagram envelope с максимальным plaintext frame 1080 bytes.
В этот лимит входят destination и payload.

Если пакет не помещается:

- возвращается явная `ErrDatagramTooLarge`;
- association не разрушается;
- пакет не режется молча на части.

Это особенно хорошо подходит для DNS и большинства небольших realtime/game packets.
Большие UDP payload требуют будущего versioned fragmentation/reassembly layer.

## Безопасность

- QUIC использует TLS 1.3;
- server certificate проверяется CA chain или SHA-256 pin;
- отдельный Chameleon PSK-authenticated control handshake обязателен до UDP relay;
- destination и payload QUIC datagrams дополнительно проходят через PSK-derived AEAD;
- replayed authentication nonce отклоняется;
- VPS по умолчанию блокирует private/loopback/link-local destinations так же, как TCP tunnel.

## Server resources

Одна QUIC UDP association может держать до 64 destination sockets.
Это ограничивает случайное или злонамеренное разрастание состояния.

## Совместимость

Stream QUIC ALPN 0.9.0 сохраняется.
Datagram mode использует отдельный ALPN, поэтому server 0.9.1 может обслуживать stream clients 0.9.0 и datagram clients 0.9.1 одновременно.

## Что дальше

0.9.2 добавит более глубокое автоматическое probing для UDP/IPv4/IPv6 и будет обучать выбор direct-vs-QUIC UDP на реальных результатах.
