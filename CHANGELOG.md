# Changelog

Здесь записываются заметные изменения проекта по версиям.

## [0.3.0] - 2026-09-25

### Added

- добавлен `pkg/dpi` — отдельный adaptive DPI strategy layer;
- память стратегий разделена по `network + destination + traffic class`;
- добавлены дешёвые cross-platform стратегии `direct`, `split-early` и `paced-split`;
- добавлены packet-control capabilities `fake-then-split` и `disorder-split` для будущих платформенных backend-ов;
- добавлено JSON-сохранение результатов: attempts, success/failure, failure streak, latency и throughput;
- generic writer умеет безопасно выполнять userspace split и явно отказывается имитировать raw-packet возможности без нужного backend-а;
- добавлен документ `docs/adaptive_dpi.md` с архитектурой и этапами интеграции.

### Changed

- стратегия по умолчанию теперь проектируется по принципу direct-first: если обычный путь работает, дополнительная обработка не нужна;
- будущая интеграция идей zapret/ByeDPI отделена от transport core, чтобы не привязывать весь проект к root/netfilter/VpnService;
- обучение остаётся локальным и lightweight: без нейросети, облака и дополнительных платных сервисов.

### Tests

- добавлены тесты на выбор direct без истории;
- добавлена проверка автоматической эскалации после повторных ошибок direct;
- добавлена проверка запоминания успешной стратегии;
- добавлена изоляция истории между разными сетями;
- добавлена проверка persistence после перезапуска;
- добавлены тесты userspace split writer и отказа от packet-control стратегии без backend-а.

## [0.2.1] - 2026-09-25

### Fixed

- исправлен deadlock в хранилище маршрутов при `Upsert()`;
- закрыта data race между отправкой пакетов и заменой session cipher;
- добавлена синхронизация для session memory;
- runtime-пути хранилищ больше не могут быть подменены данными из JSON;
- повреждённый постоянный Ed25519-ключ теперь вызывает ошибку, а не тихую генерацию новой identity;
- пустые ключи и passphrase больше не принимаются cipher-ом;
- entropy budget теперь действительно накопительный;
- исправлен случай, когда остаток бюджета `0` ошибочно превращался в режим «без лимита»;
- security state защищён при параллельных `Send()`.

### Changed

- минимальная версия Go поднята до 1.25;
- `golang.org/x/crypto` обновлён до 0.52.0.

### Tests

- добавлены regression-тесты на deadlock, повреждённую identity, runtime path и пустые secrets;
- добавлены race-тесты на concurrent cipher update, session memory и security state;
- добавлен отдельный тест на полное исчерпание entropy budget.

## [0.2.0] - 2026-09-25

### Added

- authenticated session bootstrap через ephemeral X25519 + persistent Ed25519;
- HKDF-SHA256 session key derivation;
- TOFU pinning для peer identity;
- режим сервера `--require-auth`;
- отдельный session cipher после успешного handshake;
- тест, подтверждающий одинаковый shared secret и session key у двух сторон.

### Fixed

- исправлен wire format identity envelope и добавлена совместимость с короткоживущим legacy-форматом;
- исправлен CI: форматирование, `go vet`, `go test -race`, build;
- Go module/import path приведён к `github.com/crakacr-alt/Chameleon-Protocol`;
- удалены случайно закоммиченные бинарники, IDE-базы и generated identity files.

### Notes

- проект остаётся исследовательским transport prototype;
- TOFU не защищает первый контакт от MITM;
- полноценный автоматический rekey state machine и независимый security audit ещё не завершены.

## [0.1.0] - 2026-07-16

### Added

- adaptive learner persistence via JSON;
- deterministic epoch-based profile rotation;
- traffic normalization with padding and jitter;
- AEAD payload encryption via AES-GCM;
- minimal X25519 key-exchange primitive for research sessions;
- transport lifecycle session state tracking;
- release-ready project README and publication guidance.

### Changed

- transport send path hardened to avoid holding the write mutex during jitter sleep;
- profile resolution now preserves configured transport profile.

### Notes

- protocol remains a research prototype and is not a full production-ready adversarial transport system.
