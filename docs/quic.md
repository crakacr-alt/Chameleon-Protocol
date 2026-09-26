# QUIC carrier — Chameleon 0.9.0

## Зачем он нужен

До 0.9 основной рабочий tunnel path Chameleon был TCP/TLS.

В 0.9 добавлен настоящий QUIC transport поверх UDP. Это даёт planner ещё один
независимый путь, который может вести себя лучше на сети, где TCP и UDP
фильтруются или шейпятся по-разному.

## Архитектура

```text
application
   ↓
local SOCKS5
   ↓
Adaptive Planner
   ├── direct TCP
   ├── Chameleon QUIC  ← UDP
   ├── Chameleon TLS   ← TCP
   ├── raw Chameleon TCP
   └── external relay
```

QUIC carrier не заменяет Chameleon authentication.

Внутри QUIC bidirectional stream всё ещё выполняется Chameleon tunnel handshake:

```text
QUIC / TLS 1.3
      ↓
Chameleon encrypted hello
      ↓
destination hidden inside AEAD
      ↓
per-connection tunnel key
      ↓
encrypted framed stream
```

Таким образом, транспорт и Chameleon security layer остаются независимыми.

## Сервер

Installer 0.9 использует одинаковый номер порта:

```text
443/tcp  → TLS front
443/udp  → QUIC
```

Если 443 занят существующим web server:

```text
9443/tcp
9443/udp
```

TCP и UDP не конфликтуют между собой.

## Клиент

Пример:

```bash
export CHAMELEON_TUNNEL_PSK='...'

chameleon-proxy \
  --chameleon-quic=SERVER:443 \
  --chameleon-tls=SERVER:443 \
  --tls-fingerprint=SHA256_PIN
```

Planner сам выбирает путь.

## Unknown-path race

На новой сети Chameleon ещё не знает, что быстрее.

Вместо последовательного ожидания нескольких timeout используется небольшая
staggered race двух самых дешёвых carrier:

```text
t=0 ms      direct starts
t=150 ms    next carrier starts, only if direct hasn't won
```

Первое успешное соединение выигрывает. Лишнее успешное соединение закрывается.

Как только появляются реальные observations, обычный learned planner снова
становится главным.

## Старение памяти

Сеть может измениться.

Поэтому carrier evidence имеет half-life 24 часа. Старые ошибки постепенно
теряют влияние, а score возвращается к базовой стоимости carrier.

Это позволяет Chameleon спустя время снова проверить дешёвый direct route,
вместо вечного использования fallback после старой блокировки.

## DPI и QUIC

`split-early` и `paced-split` в текущем виде являются TCP/userspace
first-write techniques.

Chameleon не применяет их к QUIC для вида.

Для QUIC 0.9.0 используется direct DPI policy. Packet-level UDP techniques
должны появиться только в отдельном backend, который действительно управляет
UDP packets.

## PMTU

QUIC stack использует собственный Path MTU Discovery. Chameleon не задаёт
искусственный MTU поверх него без измерений.

## Что именно готово в 0.9.0

Готово:

- реальный QUIC over UDP;
- TLS 1.3;
- certificate CA verification или SHA-256 pin;
- Chameleon authenticated stream внутри QUIC;
- automatic carrier selection;
- first-use carrier racing;
- TCP+UDP server deployment;
- persistence с time decay;
- QUIC end-to-end integration test.

Не заявляется готовым в 0.9.0:

- SOCKS5 UDP ASSOCIATE;
- arbitrary application UDP datagrams;
- QUIC datagram proxy executor.

Это запланировано в 0.9.1.
