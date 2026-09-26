# Chameleon Protocol

Chameleon Protocol — исследовательский адаптивный transport-стек для нормализации сетевых потоков. Цель проекта: обеспечить управляемую непредсказуемость трафика для тестирования устойчивости сетевых фильтров и классификаторов. Текущая версия включает экспериментальный authenticated handshake (Ed25519 + X25519), TOFU-пиннинг identity, peer-shared session key через HKDF, persistent identity/route stores, state-sync для ротации профилей по epoch и стабильный pkg/normalizer API.

## Что уже сделано

- морфинг трафика по профилям WebRTC, HTTP/3 и Gaming
- рандомизированная нормализация размеров пакетов через padding
- ограниченный jitter для управления таймингами
- детерминированная ротация профилей по epoch
- AEAD-защита полезной нагрузки через AES-GCM
- lightweight learner с сохранением решений в JSON
- adaptive DPI Strategy Engine: direct-first выбор стратегии по сети, назначению и типу трафика
- cross-platform userspace split/paced-split без обязательного root-доступа
- NetworkContext detector, который отличает физическую сеть от Tailscale/WireGuard/tun
- Carrier Engine: direct / Chameleon UDP / Chameleon TCP / внешний relay
- Combined Adaptive Planner с раздельным обучением DPI-сбоев и недоступных маршрутов
- реальный local SOCKS5 proxy без системного TUN/VPN
- encrypted Chameleon TCP tunnel до VPS как рабочий fallback carrier
- TLS-fronted Chameleon carrier с certificate pinning и decoy HTTPS response
- real QUIC carrier поверх UDP с TLS 1.3 и тем же Chameleon authenticated tunnel
- first-use adaptive race: direct стартует первым, fallback подключается только если direct не успел быстро победить
- time decay маршрутов: старая блокировка постепенно перестаёт диктовать выбор
- one-command VPS installer + hardened systemd + automatic health monitor
- внешний SOCKS5 relay/sidecar как ещё один optional fallback
- автоматическое распознавание early EOF/RST/blackhole после успешного TCP connect
- direct circuit breaker: после провала всех DPI strategies новый сеанс временно уходит на tunnel/relay
- долговременная session memory для накопления опыта профиля между запуском и сессиями
- воспроизводимый benchmark и отчёт по метрикам
- базовый session lifecycle и минимальный handshake через X25519

Что улучшено в этом релизе:

- Добавлен authenticated handshake: Ed25519 подписывает ephemeral X25519 public key, после чего обе стороны получают один X25519 shared secret и выводят session key через HKDF.
- Добавлен pkg/identity с TOFU-пиннингом: первый ключ сохраняется, а тихая подмена уже известной identity отклоняется.
- Добавлен pkg/storage: персистентный store маршрутов (destination -> profile) для долгосрочной оптимизации.
- Добавлен pkg/normalizer: стабильный API-обёртка над core.Normalizer.
- Расширен state.Sync с EpochID для детерминированного derivation и ProfileAt.
- cmd/server поддерживает authenticated session bootstrap и опциональный `--require-auth`, запрещающий data path до успешного handshake.

## Чем проект отличается

Chameleon Protocol исследует не только шифрование и передачу данных, а управляемое изменение наблюдаемого поведения потока. Проект объединяет transport prototype, воспроизводимый benchmark и адаптивную память, которая:

- сам анализирует, какой профиль поведения даёт лучший результат
- сам запоминает успешные и неуспешные сценарии
- сам усиливает устойчивость к классификаторам через смену профилей по epoch
- сам строит воспроизводимую матрицу сравнения профилей и отдаёт evidence по реальным результатам

Именно это и является ключевым инновационным преимуществом: не “ещё один VPN слой”, а учебный и самообучающийся транспортный стек, работающий на уровне поведения потока.

## Цели проекта

Протокол разрабатывается для научных исследований в области:

1. нормализации потока для маскировки сетевых сигнатур
2. адаптивного выбора профиля при изменении сетевых условий
3. имитации легитимной активности для оценки устойчивости к классификаторам трафика
4. воспроизводимых локальных экспериментов поверх UDP transport

## Результаты влияния Chameleon Protocol:

Цель: измерить влияние padding и jitter на throughput и latency при трёх профилях: webrtc, http3, gaming.

Методика
- Используется локальный benchmark (`cmd/benchmark`) с настраиваемым burst/rounds/payload.
- Для каждой конфигурации прогоняется N=3 повторов, собираются: mean latency, mean throughput, loss.
- Сохраняется конфигурация профиля и случайная seed для воспроизводимости.

Ключевые выводы
- Padding повышает payload entropy, но снижает throughput линейно с добавленным overhead.
- Jitter помогает замаскировать интервалы пакетов, но увеличивает mean latency; оптимальные значения зависят от профиля.

Таблица результатов

| Profile | Padding range | Mean Latency | Throughput |
|---|---:|---:|---:|
| webrtc | 32-128 | 5.6ms | 2.69KB/s |
| http3 | 16-96 | 3.3ms | 4.45KB/s |
| gaming | 64-256 | 5.5ms | 2.69KB/s |


## Архитектура

```text
chameleon-protocol/
├── cmd/
│   ├── client/      # клиентский вход
│   ├── server/      # серверный вход
│   └── benchmark/   # бенчмарк-скрипт
├── pkg/
│   ├── adaptive/    # обучение и сохранение маршрутов
│   ├── core/        # транспортная обёртка и кадрирование
│   ├── crypto/      # AEAD и key-exchange примитивы
│   ├── dpi/         # adaptive DPI strategy engine и userspace executor
│   ├── carrier/     # обучение и выбор маршрута/carrier
│   ├── networkctx/  # контекст текущей физической сети
│   ├── planner/     # объединённый carrier + DPI decision engine
│   ├── traffic/     # классификация web/streaming/realtime/bulk
│   ├── tunnel/      # encrypted authenticated TCP tunnel
│   ├── socks5/      # local SOCKS server + external relay client
│   ├── proxy/       # выполнение adaptive carrier plan
│   ├── experiment/  # сценарии и метрики
│   ├── morph/       # padding и jitter
│   └── state/       # детерминированная синхронизация epoch
└── go.mod
```

## Основные компоненты

### pkg/core

- Transport: UDP-обёртка над нормализацией и отправкой
- Normalizer: случайная нормализация целевого размера данных
- BehaviorProfile: поверхность выбора профиля поведения
- EncodeFrame / DecodeFrame: упаковка и валидация кадра

### pkg/morph

- PaddingConfig: задаёт окно случайного дополнения длины пакета
- JitterConfig: управляет ограниченным delay для тайминга

### pkg/crypto

- Cipher: симметричная AEAD-обёртка над AES-GCM
- KeyExchange: минимальная X25519-выработка общего секрета
- Handshake: базовый session bootstrap для исследования

### pkg/state

- Sync: детерминированное отображение профиля по общему секрету и epoch
- EpochState: строгий контроллер ротации по времени
- Session: явный lifecycle состояния сессии
- SessionContext: epoch-bound key derivation context для controlled rekey policy

### pkg/adaptive

- Learner: lightweight scoring-память с сохранением истории и выбором профиля

### pkg/dpi

- Engine: хранит опыт отдельно для network + destination + traffic class
- Strategy: direct/split/paced-split и capability-модель для packet-level backend
- ApplyWriter: дешёвый userspace executor для proxy/embedded режима
- JSON persistence: локальная память успешных и неуспешных стратегий

Подробности: [docs/adaptive_dpi.md](docs/adaptive_dpi.md)

### pkg/carrier

- Engine: выбирает direct/tunnel/relay по реальному опыту конкретной сети
- отдельные веса для interactive, streaming и bulk traffic
- JSON persistence между запусками
- внешний relay можно использовать как optional fallback

### pkg/networkctx

- best-effort определение текущей физической сети
- отдельная классификация virtual interfaces, чтобы не конфликтовать с Tailscale/WireGuard
- privacy-friendly NetworkID fingerprint

### pkg/planner

- объединяет Carrier Engine и DPI Engine
- различает DPI failure и carrier failure
- при туннеле обучает DPI на видимом carrier endpoint, а не на конечном сайте

Подробности: [docs/adaptive_planner.md](docs/adaptive_planner.md)

### pkg/tunnel

- encrypted TCP carrier с per-connection HKDF key
- AES-GCM framing всего stream
- destination/timestamp скрыты внутри AEAD handshake
- replay cache и clock-skew validation
- private VPS egress заблокирован по умолчанию

### pkg/socks5

- локальный SOCKS5 CONNECT server
- SOCKS5 client для existing sidecar/relay
- proxy по умолчанию слушает только loopback

### pkg/proxy

- исполняет Adaptive Planner
- автоматически replans после hard carrier failure
- записывает carrier success после TCP connect
- записывает DPI success сразу после первого реального response byte
- автоматически учится на early EOF/RST/first-response timeout
- временно выключает direct после недавнего провала всех userspace DPI strategies
- first-write DPI strategy не добавляет overhead ко всему потоку

Подробности: [docs/socks_tunnel.md](docs/socks_tunnel.md)

### QUIC carrier

Начиная с 0.9 сервер может одновременно принимать TLS/TCP и QUIC/UDP на одном номере порта.
QUIC переносит надёжные proxy streams через настоящий UDP transport и участвует в adaptive selection.

Подробности: [docs/quic.md](docs/quic.md)

### UDP data path

SOCKS5 UDP ASSOCIATE поддерживает direct UDP и QUIC DATAGRAM.
Режим выбирается через `--udp-mode=auto|direct|quic`.

Подробности: [docs/udp.md](docs/udp.md)

### pkg/experiment

- Scenario: воспроизводимый benchmark поверх loopback UDP
- Metrics: отчёт по throughput, latency, loss rate и entropy

## Поддерживаемые профили

- webrtc
- http3
- gaming

Каждый профиль задаёт свои значения padding и jitter.

## Примечание по безопасности

Это **исследовательский прототип, а не production VPN**. Текущая реализация использует:

- AES-GCM для AEAD-защиты payload;
- Ed25519-аутентификацию ephemeral X25519 public keys;
- общий X25519 secret и HKDF-derived session key;
- TOFU-пиннинг peer identity с запретом тихой подмены сохранённого ключа;
- опциональный режим `--require-auth`, который отклоняет data packets до handshake;
- race-тесты, `go vet` и CI build.

Ограничения:

- первая встреча в модели TOFU остаётся уязвимой для активного MITM, если fingerprint не сверяется по отдельному доверенному каналу;
- полноценный автоматический rekey state machine ещё не подключён к data path;
- проект не проходил независимый криптографический аудит;
- padding/jitter не гарантируют устойчивость против реальных traffic classifiers.

Поэтому проект следует использовать как экспериментальную платформу и учебно-исследовательский transport stack, а не как замену WireGuard/TLS/QUIC в production.

## Быстрый старт

### Требования

- Go 1.27+
- Linux, macOS или Windows с обычной Go toolchain

### Сервер: one-command VPS install

На Ubuntu/Debian достаточно:

```bash
git clone https://github.com/crakacr-alt/Chameleon-Protocol.git
cd Chameleon-Protocol
sudo ./deploy/install-server.sh
```

Installer сам создаёт PSK, TLS certificate/pin, systemd service и health timer.
На новой установке используется TCP/443, если порт свободен; иначе 9443.

После установки:

```bash
chameleonctl status
sudo chameleonctl client
sudo chameleonctl health
```

Повторный запуск installer обновляет бинарники, сохраняя существующие secrets.
Для managed checkout доступно:

```bash
sudo chameleonctl update
```

Подробнее: [docs/server_install.md](docs/server_install.md)

### Клиент

Authenticated session:

```bash
go run ./cmd/client \
  --target=127.0.0.1:9000 \
  --identity=client1 \
  --send-handshake \
  --profile=webrtc \
  --burst=3
```

Для исследовательской совместимости остаётся legacy PSK-режим:

```bash
go run ./cmd/client --target=127.0.0.1:9000 --profile=webrtc --burst=3 --psk=research-secret
```

### Локальный SOCKS5 + TCP tunnel

На VPS:

```bash
export CHAMELEON_TUNNEL_PSK="$(openssl rand -hex 32)"
go run ./cmd/tunnel-server --listen=:9443
```

На клиенте:

```bash
go run ./cmd/proxy \
  --listen=127.0.0.1:1080 \
  --chameleon-tcp=SERVER_IP:9443
```

После этого приложение может использовать SOCKS5 `127.0.0.1:1080`.
Системный VPN-интерфейс Chameleon в этом режиме не создаёт.

Proxy автоматически измеряет early response failures. По умолчанию first-response
window составляет 12 секунд, а direct cooldown после исчерпания DPI strategies —
10 минут. При необходимости:

```bash
go run ./cmd/proxy \
  --chameleon-tcp=SERVER_IP:9443 \
  --failure-window=8s \
  --direct-cooldown=5m
```

Для постоянного VPS deployment используйте:

```bash
sudo ./deploy/install-server.sh
```

Installer автоматически включает TLS-front, systemd restart и health monitor.

Подробнее: [docs/server_install.md](docs/server_install.md) и [docs/socks_tunnel.md](docs/socks_tunnel.md)

### Adaptive planner

Показать план для текущей сети:

```bash
go run ./cmd/plan \
  --destination=example.com:443 \
  --protocol=tcp \
  --purpose=web \
  --chameleon-udp=YOUR_SERVER:9000 \
  --chameleon-tcp=YOUR_SERVER:443
```

Существующий Tailscale/DERP или другой sidecar можно передать как optional relay:

```bash
go run ./cmd/plan --destination=example.com:443 --relay=127.0.0.1:1080
```

### Бенчмарк

```bash
go run ./cmd/benchmark --profile=webrtc --burst=2 --rounds=1 --payload=hello-chameleon --psk=research-secret
```

### Сравнение профилей

```bash
go run ./cmd/benchmark --compare --payload=hello-chameleon --burst=1 --rounds=1 --psk=research-secret
```

Пример вывода:

```text
profile comparison
webrtc: throughput=2691.11 loss=0.0000 mean_latency=5.5739ms
http3: throughput=4450.25 loss=0.0000 mean_latency=3.3706ms
gaming: throughput=2691.16 loss=0.0000 mean_latency=5.5738ms
```

### Полная проверка

```bash
go test ./...
```

2. Установить зависимости и запустить deployment:

```bash
apt update
apt install -y git golang-go
go build -v -o chameleon-server ./cmd/server/main.go
chmod +x /opt/chameleon-protocol/deploy/deploy-vps.sh
/opt/chameleon-protocol/deploy/deploy-vps.sh
systemctl status chameleon-server --no-pager
```

3. Проверить, что сервис слушает UDP-порт:

```bash
ss -lunp | grep 9000
```

### What is ready / what is still next

**Ready now**
- local SOCKS5 proxy + encrypted TCP/TLS tunnel fallback
- TLS front with certificate pinning and decoy HTTPS response
- adaptive direct / tunnel / external relay selection
- one-command Linux VPS install with hardened systemd and health timer
- adaptive learning and session memory
- benchmark comparison matrix

**Still next**
- session resume across carrier switches
- fingerprint UX и optional configured trust anchors поверх TOFU
- full epoch key rekey state machine, интегрированный в data path
- fuzzing и независимый security review
- реальные desktop/mobile clients
- long-term resilience tests против независимых classifiers

## Статус

Проект уже представляет собой рабочий исследовательский transport prototype с хорошей модульной структурой, стабильным тестовым покрытием, адаптивной памятью и воспроизводимым benchmark evidence.

Дальнейшая работа должна идти в сторону:

- проверяемого trust bootstrap (fingerprints / configured trust anchors);
- полноценного key lifecycle и rekey state machine;
- fuzzing, fault injection и независимого security review;
- benchmark-набора с внешними traffic classifiers и сырыми reproducible results;
- реальных desktop/mobile clients.

---

## Версии и roadmap

Текущая версия хранится в `VERSION`, а runtime binaries поддерживают `--version`.
Правила выпуска: [docs/versioning.md](docs/versioning.md).
План развития до 2.0: [ROADMAP.md](ROADMAP.md).

---

# English Summary

Chameleon Protocol is a research-oriented adaptive transport stack for network-flow normalization. The goal is to shape traffic to look like different legitimate network profiles while keeping packet overhead, reproducibility, and experimentability under control.

## What is already implemented

- traffic morphing across WebRTC, HTTP/3, and Gaming profiles
- randomized packet-size normalization through padding
- bounded jitter control for timing shaping
- deterministic epoch-based profile rotation
- AEAD payload protection using AES-GCM
- a lightweight learner that stores policy decisions in JSON
- durable session memory for profile-performance learning across runs
- reproducible benchmarks and metrics reporting
- a basic session lifecycle and minimal X25519-based handshake

## What makes the project different

Chameleon Protocol focuses on measurable flow shaping rather than claiming to replace established secure transports. It combines an experimental transport, reproducible benchmarks, and adaptive state that:

- analyzes which behavior profile performs best in real conditions
- remembers successful and failed pattern outcomes
- rotates profile behavior by epoch to reduce direct traffic signature predictability
- produces benchmark evidence rather than only theoretical claims

This makes the protocol more than “another VPN layer”: it becomes an evidence-driven, behavior-adaptive transport research platform.

## Quick start

### Server

```bash
go run ./cmd/server --address=127.0.0.1:9000 --require-auth
```

### Client

```bash
go run ./cmd/client --target=127.0.0.1:9000 --identity=client1 --send-handshake --profile=webrtc --burst=3
```

### Benchmark comparison

```bash
go run ./cmd/benchmark --compare --payload=hello-chameleon --burst=1 --rounds=1 --psk=research-secret
```

### Full verification

```bash
go test ./...
```

### VPS deployment

The release deployment bundle is already prepared in the `deploy/` directory.

1. Connect to the VPS:

```bash
ssh root@YOUR_VPS_IP
```

2. Install dependencies and run the deployment script:

```bash
apt update
apt install -y git golang-go
chmod +x /opt/chameleon-protocol/deploy/deploy-vps.sh
/opt/chameleon-protocol/deploy/deploy-vps.sh
systemctl status chameleon-server --no-pager
```

3. Confirm the UDP listener is up:

```bash
ss -lunp | grep 9000
```

## Current status

The project is already a working research transport prototype with a stable modular structure, passing tests, adaptive memory, and benchmark-backed evidence. The next logical stage is deployment on a VPS as a long-running service with a hardened session and rekey policy.

## Production-grade release summary

### Ready now

- benchmark and comparison matrix CLI
- self-learning adaptive profile memory
- session-aware transport behavior
- deployment scripts for Linux VPS
- release-ready bilingual documentation

### Next engineering frontier

- explicit fingerprint UX / configured trust anchors
- fully integrated rekey policy for every epoch boundary
- fuzzing and independent security review
- real clients for mobile and desktop
- reproducible evaluation against independent traffic classifiers
