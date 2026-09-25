# Changelog

Здесь записываются заметные изменения проекта по версиям.

## [0.2.2] - 2026-09-25

### Added

- CodeQL-анализ;
- Dependabot для Go modules и GitHub Actions;
- CODEOWNERS, PR template и bug template;
- fuzz seed для `DecodeFrame`;
- VERSION-файл и проверяемый release process;
- source-only GitHub Release workflow.

### Changed

- CI переведён на актуальные official GitHub Actions;
- README синхронизирован с Go 1.25 и версией 0.2.2;
- CONTRIBUTING требует vet, race tests и build перед PR;
- SECURITY описывает конкретные security-sensitive классы ошибок.

### Notes

- transport/crypto функциональность в 0.2.2 не расширялась;
- проект остаётся исследовательским прототипом.

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
