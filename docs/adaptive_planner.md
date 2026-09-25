# Adaptive Planner v0.4.0

Chameleon теперь разделяет две разные задачи:

1. **как добраться до следующего узла** — Carrier Engine;
2. **как вести себя на этом пути** — DPI Strategy Engine.

Это важно, потому что сбой уровня DPI и недоступный IP — не одно и то же.

## Поток принятия решения

```text
Network Context
      +
Destination / Protocol / Purpose
      |
      v
Traffic Classifier
      |
      +----------------------+
      |                      |
      v                      v
Carrier Engine          DPI Strategy Engine
      |                      |
      +----------+-----------+
                 v
            Combined Plan
```

## Network Context

`pkg/networkctx` делает best-effort определение текущей сети без root-доступа.

Он различает:

- Ethernet;
- Wi-Fi;
- mobile;
- virtual/tunnel interfaces;
- unknown.

Tailscale, WireGuard, tun/tap, Wintun и похожие интерфейсы считаются виртуальными
и получают низкий приоритет при выборе identity сети. Это нужно, чтобы Chameleon
не учился на самом VPN-интерфейсе вместо физического подключения.

В постоянную память не записывается полный локальный IP. Для NetworkID используется
короткий SHA-256 fingerprint от грубого сетевого контекста.

## Traffic Classifier

`pkg/traffic` делит соединения на:

- `web`;
- `interactive`;
- `streaming`;
- `bulk`;
- `realtime`;
- `default`.

Caller может явно передать purpose. Если его нет, используется консервативная
классификация по protocol/port.

## Carrier Engine

`pkg/carrier` запоминает работу маршрутов отдельно по:

```text
network + destination + traffic class + protocol
```

Поддерживаемые типы carrier:

- `direct`;
- `chameleon-udp`;
- `chameleon-tcp`;
- `relay`.

Relay специально сделан внешним capability. Так существующий Tailscale/DERP,
SOCKS sidecar или другой relay можно подключать как fallback без жёсткой
зависимости Chameleon от конкретного продукта.

### Разные цели — разные веса

Для игр и realtime сильнее штрафуется latency.

Для streaming важнее стабильный throughput.

Для bulk/download throughput весит ещё больше.

Это лучше одной общей формулы "самый быстрый маршрут".

## DPI failure и carrier failure

Самое важное правило версии 0.4.0:

**не каждый сетевой сбой должен менять маршрут.**

Если соединение с endpoint установилось, но дальше сессию ломает DPI, результат
пишется с `ScopeDPI`.

Тогда:

- carrier остаётся успешным;
- DPI strategy получает failure;
- следующий план может оставить `direct`, но перейти с `direct` DPI strategy
  на `split-early`.

Если сам endpoint не достигается, используется `ScopeCarrier`:

- DPI strategy не штрафуется;
- Carrier Engine ищет следующий маршрут.

Если слой неизвестен, используется `ScopeBoth`.

Это уменьшает ненужные переходы на relay/DERP и сохраняет скорость.

## CLI

Посмотреть план:

```bash
go run ./cmd/plan \
  --destination=example.com:443 \
  --protocol=tcp \
  --purpose=web \
  --chameleon-udp=SERVER_IP:9000 \
  --chameleon-tcp=SERVER_IP:443
```

Добавить существующий relay:

```bash
go run ./cmd/plan \
  --destination=example.com:443 \
  --relay=127.0.0.1:1080
```

Записать реальный успешный результат выбранного плана:

```bash
go run ./cmd/plan \
  --destination=example.com:443 \
  --record=success \
  --latency-ms=45 \
  --throughput=5000000
```

Записать DPI-сбой, не штрафуя carrier:

```bash
go run ./cmd/plan \
  --destination=example.com:443 \
  --record=failure \
  --scope=dpi \
  --latency-ms=40 \
  --failure="reset after connection established"
```

Записать недоступность маршрута:

```bash
go run ./cmd/plan \
  --destination=example.com:443 \
  --record=failure \
  --scope=carrier \
  --failure="endpoint timeout"
```

По умолчанию состояние хранится в пользовательском config directory в каталоге
`chameleon`.

## Что ещё не делает v0.4.0

Planner уже рабочий и сохраняет решения, но пока execution layer не выполняет
автоматическую гонку carrier-ов сам.

Следующий этап:

1. active probes;
2. автоматическое определение failure scope по стадии соединения;
3. proxy/sidecar executor;
4. carrier racing;
5. session handoff;
6. optional Linux packet backend;
7. интеграция Chameleon UDP/TCP carrier с реальным relay data path.

Главное ограничение остаётся физическим: если один IP полностью недостижим,
нужен хотя бы один альтернативный достижимый next hop.
