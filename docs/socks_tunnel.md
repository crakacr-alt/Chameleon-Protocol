# Chameleon SOCKS + TCP Tunnel (v0.5.0)

Версия 0.5.0 добавляет первый реальный general-purpose proxy data path.

До этого `cmd/server` был исследовательским UDP echo transport. Теперь отдельно
добавлены:

- `cmd/proxy` — локальный SOCKS5 proxy;
- `cmd/tunnel-server` — TCP tunnel endpoint на VPS;
- `pkg/proxy` — исполнение Adaptive Planner;
- `pkg/tunnel` — аутентифицированный шифрованный TCP stream;
- `pkg/socks5` — локальный SOCKS5 server и SOCKS5 relay client.

## Схема

```text
Browser / app
     |
     | SOCKS5 127.0.0.1:1080
     v
Chameleon proxy
     |
     +---- direct -----------------------> destination
     |
     +---- encrypted Chameleon TCP -----> VPS tunnel -----> destination
     |
     +---- existing SOCKS relay --------> relay ----------> destination
```

Planner выбирает carrier по истории конкретной сети и назначения.

## Почему это не конфликтует с другим VPN

`cmd/proxy` по умолчанию слушает только `127.0.0.1:1080`.

Он не создаёт TUN-интерфейс, не меняет default route и не занимает Android
`VpnService`. Поэтому его можно использовать рядом с Tailscale/WireGuard или
другим VPN, если приложение умеет SOCKS5.

Внешний SOCKS5 sidecar можно добавить как fallback через `--relay-socks`.

## Запуск VPS

Сгенерировать отдельный PSK:

```bash
export CHAMELEON_TUNNEL_PSK="$(openssl rand -hex 32)"
```

Для ручного запуска:

```bash
go run ./cmd/tunnel-server \
  --listen=:9443 \
  --psk="$CHAMELEON_TUNNEL_PSK"
```

Private/loopback egress VPS по умолчанию запрещён.

Если это действительно нужно для собственной лабораторной сети:

```bash
go run ./cmd/tunnel-server \
  --listen=:9443 \
  --psk="$CHAMELEON_TUNNEL_PSK" \
  --allow-private
```

## systemd

На VPS:

```bash
export CHAMELEON_TUNNEL_PSK="$(openssl rand -hex 32)"
sudo -E ./deploy/install-tunnel.sh
```

PSK сохраняется в:

```text
/etc/chameleon/tunnel.env
```

Файл создаётся с правами `0600`.

## Запуск локального proxy

Только direct:

```bash
go run ./cmd/proxy
```

Direct + Chameleon TCP fallback:

```bash
go run ./cmd/proxy \
  --listen=127.0.0.1:1080 \
  --chameleon-tcp=SERVER_IP:9443 \
  --psk="$CHAMELEON_TUNNEL_PSK"
```

Добавить уже существующий локальный SOCKS relay:

```bash
go run ./cmd/proxy \
  --chameleon-tcp=SERVER_IP:9443 \
  --psk="$CHAMELEON_TUNNEL_PSK" \
  --relay-socks=127.0.0.1:1081
```

После этого в браузере или приложении:

```text
SOCKS5 host: 127.0.0.1
SOCKS5 port: 1080
```

## Adaptive behavior

При новом network context:

1. planner предпочитает direct;
2. если TCP route не открывается, failure записывается как carrier failure;
3. planner автоматически выбирает следующий carrier;
4. successful TCP dial помечает только carrier как working;
5. DPI strategy считается успешной только после получения реальных response bytes;
6. learned state сохраняется в user config directory.

Так Chameleon не объявляет DPI «пройденным» только потому, что TCP handshake
состоялся.

## First-write DPI layer

Для direct connection userspace strategy применяется только к первому `Write()`.

Для Chameleon TCP tunnel strategy применяется к первому hop до VPS — то есть к
тому соединению, которое видит локальная сеть.

После первого write дальнейший поток идёт без split overhead.

Это особенно важно для streaming/download: workaround не должен резать каждый
кусок большого потока.

## Tunnel handshake

Tunnel hello не содержит destination в открытом виде.

Wire shape:

```text
[random KDF salt][encrypted length][AEAD ciphertext]
```

В AEAD ciphertext находятся:

- version;
- timestamp;
- destination.

Ключ на соединение выводится через HKDF-SHA256 из PSK и случайного salt.

Сервер:

- отклоняет неверный AEAD;
- проверяет clock skew;
- хранит replay cache случайных salt;
- не отвечает валидным protocol banner на неверную аутентификацию.

После handshake весь TCP stream идёт в отдельных AES-GCM frames.

## Что v0.5.0 пока не решает

TCP tunnel уже функционален, но это ещё не финальная маскировка под обычный
HTTPS/HTTP2 traffic.

Следующие этапы:

- стандартный TLS/HTTP carrier поверх TCP tunnel;
- active application probes для автоматического DPI failure classification;
- UDP/QUIC general-purpose relay;
- session resume при смене carrier;
- Android wrapper и desktop service;
- optional packet-level backend для Linux/router.

То есть v0.5.0 — первый рабочий proxy path, а не заявление о полной
неразличимости от обычного web traffic.
