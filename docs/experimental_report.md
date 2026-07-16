# Chameleon Protocol — Experimental Report

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

Limitations
- Локальные замеры не учитывают сетевые фильтры и реальные классификаторы; для полной валидации требуется A/B тестирование через реальные middleboxes и ML-классификаторы.

Recommendations for stronger robustness
- Integrate traffic shaping policies that include protocol-specific stateful behaviors (e.g., TLS handshakes pacing).
- Use stronger authenticated handshake with identity attestation and optional PKI.
- Introduce an entropy budget controller to avoid pathological padding that creates obvious patterns.
