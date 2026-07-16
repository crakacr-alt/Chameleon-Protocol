# Chameleon Protocol

Chameleon Protocol — это исследовательский адаптивный транспортный стек для нормализации сетевых потоков. Проект направлен на то, чтобы динамически маскировать трафик под различные сетевые шаблоны, сохраняя при этом низкую перегрузку, воспроизводимость и поддерживаемость для экспериментов с устойчивостью протокола к классификации трафика.

## Что уже сделано

- морфинг трафика по профилям WebRTC, HTTP/3 и Gaming
- рандомизированная нормализация размеров пакетов через padding
- ограниченный jitter для управления таймингами
- детерминированная ротация профилей по epoch
- AEAD-защита полезной нагрузки через AES-GCM
- lightweight learner с сохранением решений в JSON
- долговременная session memory для накопления опыта профиля между запуском и сессиями
- воспроизводимый benchmark и отчёт по метрикам
- базовый session lifecycle и минимальный handshake через X25519

## Почему это должно быть лучше всех существующих

Существующие решения в основном ориентированы на:

1. шифрование и маршрутизацию
2. стабильный транспортный туннель
3. маскирование трафика только на уровне грубых паттернов

Chameleon Protocol должен отличаться тем, что он не просто транспорт, а адаптивный протокол, который:

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

### pkg/experiment

- Scenario: воспроизводимый benchmark поверх loopback UDP
- Metrics: отчёт по throughput, latency, loss rate и entropy

## Поддерживаемые профили

- webrtc
- http3
- gaming

Каждый профиль задаёт свои значения padding и jitter.

## Примечание по безопасности

Это исследовательский прототип, а не промышленный secure transport. В текущем виде проект использует:

- AEAD-шифрование полезной нагрузки
- детерминированную ротацию epoch
- явный session lifecycle и epoch-bound key derivation
- lightweight adaptive heuristic для выбора маршрута

Это снижает прямую узнаваемость трафика, но не делает протокол невосприимчивым к статистическому анализу. Более стойкая архитектура потребует:

- аутентифицированного handshake
- жестко заданного state machine для ключей и epoch
- явного rekey-процесса при смене сессий
- контроля entropy budget для padding
- отдельной оценки под атакующими классификаторами трафика

## Быстрый старт

### Требования

- Go 1.22+
- Linux, macOS или Windows с обычной Go toolchain

### Сервер

```bash
go run ./cmd/server --address=127.0.0.1:9000 --psk=research-secret
```

### Клиент

```bash
go run ./cmd/client --target=127.0.0.1:9000 --profile=webrtc --burst=3 --psk=research-secret
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


## Статус

Проект уже представляет собой рабочий исследовательский transport prototype с хорошей модульной структурой, стабильным тестовым покрытием, адаптивной памятью и воспроизводимым benchmark evidence.

Дальнейшая работа должна идти в сторону:

- полноценного authenticated handshake
- реального key lifecycle и rekey state machine
- более строгого контроля entropy и anti-classification поведения
- расширения session context до полноценной security context для всех профилей и эпох
- деплоя на VPS и запуска как long-running transport service

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

## Why this should be better than existing approaches

Most current solutions focus on transport encryption, routing, or simple traffic camouflage. Chameleon Protocol is different because it aims to become an adaptive, self-learning transport layer that:

- analyzes which behavior profile performs best in real conditions
- remembers successful and failed pattern outcomes
- rotates profile behavior by epoch to reduce direct traffic signature predictability
- produces benchmark evidence rather than only theoretical claims

This makes the protocol more than “another VPN layer”: it becomes an evidence-driven, behavior-adaptive transport research platform.

## Quick start

### Server

```bash
go run ./cmd/server --address=127.0.0.1:9000 --psk=research-secret
```

### Client

```bash
go run ./cmd/client --target=127.0.0.1:9000 --profile=webrtc --burst=3 --psk=research-secret
```

### Benchmark comparison

```bash
go run ./cmd/benchmark --compare --payload=hello-chameleon --burst=1 --rounds=1 --psk=research-secret
```

### Full verification

```bash
go test ./...
```

## Current status

The project is already a working research transport prototype with a stable modular structure, passing tests, adaptive memory, and benchmark-backed evidence. The next logical stage is deployment on a VPS as a long-running service with a hardened session and rekey policy.
