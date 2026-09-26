# Changelog

Здесь записываются заметные изменения проекта по версиям.

## [0.8.0] - 2026-09-26

### Added

- one-command VPS installer: `sudo ./deploy/install-server.sh`;
- автоматическая установка базовых пакетов и Go 1.25 при необходимости;
- автоматическая генерация 256-bit tunnel PSK;
- автоматическая генерация pinned self-signed ECDSA TLS certificate, если пользователь не передал свой;
- автоматический выбор TCP/443 для новой установки с fallback на 9443, если 443 уже занят;
- отдельный системный пользователь `chameleon`;
- hardened systemd unit с минимальной capability для bind к 443;
- `chameleon-health.timer` с локальным TLS probe и автоматическим restart при сбое;
- `chameleonctl` для status/logs/client/health/restart/update;
- root-only client profile с endpoint, PSK и TLS fingerprint;
- best-effort настройка активного UFW/firewalld;
- deployment shell syntax теперь проверяется в CI;
- добавлена подробная инструкция `docs/server_install.md`.

### Changed

- повторный запуск installer сохраняет существующие PSK и TLS certificate;
- legacy `install-tunnel.sh` перенаправляет на новый installer;
- server listen address и decoy file могут задаваться через environment;
- upgrade активного systemd service теперь делает restart, а не оставляет старый процесс;
- сгенерированный certificate не содержит Chameleon в subject.

### Notes

- server-side TCP/TLS deployment теперь рассчитан на постоянную эксплуатацию через systemd;
- arbitrary client traffic всё ещё требует local SOCKS/embedded/router entry point;
- UDP/QUIC general-purpose path и полноценные Android/Windows clients остаются следующими этапами.

## [0.7.0] - 2026-09-26

### Added

- TLS-fronted Chameleon tunnel carrier;
- certificate SHA-256 pinning для private/self-signed deployments;
- normal CA verification для пользовательского certificate;
- decoy HTTP response для обычных HTTPS probes;
- adaptive `chameleon-tls` carrier с приоритетом перед raw TCP fallback;
- DPI split может применяться к реальному TLS ClientHello;
- integration tests TLS handshake, pinning, decoy и adaptive proxy.

### Changed

- raw Chameleon TCP остаётся отдельным fallback carrier;
- TLS front использует обычный TLS stack, а Chameleon handshake идёт внутри него.

## [0.6.0] - 2026-09-25

### Added

- автоматическая классификация раннего DPI/application-path сбоя после уже успешного TCP connect;
- first-response deadline для direct web/streaming соединений: blackhole теперь становится измеримым timeout, а не бесконечным зависанием;
- успешный первый response byte немедленно подтверждает DPI strategy и снимает first-response deadline;
- ранние EOF/RST/EPIPE/timeout без единого response byte автоматически записываются как DPI failure;
- добавлен direct circuit breaker: если все доступные userspace DPI strategies недавно провалились, direct временно исключается и новый сеанс идёт через следующий carrier;
- direct автоматически возвращается после cooldown, поэтому старый сетевой сбой не блокирует быстрый путь навсегда;
- tunnel/relay handshake теперь считается доказательством работоспособности DPI strategy до first-hop endpoint;
- добавлены настройки proxy `--direct-cooldown` и `--failure-window`.

### Changed

- автоматическое обучение больше не ждёт закрытия успешной сессии: первый реальный response byte уже является положительным evidence;
- DPI failure после ранее записанного carrier success больше не создаёт второй carrier-success observation;
- first-response timeout применяется только к direct web/streaming трафику, а не ко всем TCP-соединениям.

### Tests

- добавлен test раннего EOF после исходящих данных;
- добавлен blackhole test, где peer принимает запрос, но не отвечает;
- добавлен test автоматического выбора следующей DPI strategy;
- добавлен test direct cooldown после провала всех DPI strategies;
- добавлена regression-проверка отсутствия duplicate carrier credit.

### Notes

- automatic recovery происходит на следующем соединении/повторной попытке приложения;
- безопасный transparent replay уже отправленных application bytes внутри существующей TCP-сессии пока не выполняется, чтобы не дублировать side-effecting запросы.

## [0.5.0] - 2026-09-25

### Added

- добавлен первый general-purpose TCP proxy data path;
- добавлен локальный `cmd/proxy` с SOCKS5 CONNECT на `127.0.0.1:1080` по умолчанию;
- добавлен `cmd/tunnel-server` для VPS;
- добавлен `pkg/tunnel` с per-connection HKDF key derivation и AES-GCM framed stream;
- tunnel hello больше не раскрывает destination: version/timestamp/destination находятся внутри AEAD ciphertext;
- добавлены timestamp + replay cache для tunnel hello;
- добавлен `pkg/socks5` с локальным server и client для existing SOCKS sidecar/relay;
- добавлен `pkg/proxy.AdaptiveDialer`, который исполняет решения Adaptive Planner и автоматически replans после hard carrier failure;
- добавлен first-write DPI wrapper: split/paced-split применяется только к первому чувствительному write, без постоянного overhead на весь поток;
- successful TCP dial теперь отдельно подтверждает carrier, а DPI success записывается только после реальных response bytes;
- VPS tunnel по умолчанию запрещает egress к loopback/private/link-local addresses;
- local SOCKS по умолчанию запрещено слушать non-loopback address без явного opt-in;
- добавлены systemd unit и `deploy/install-tunnel.sh`;
- добавлен документ `docs/socks_tunnel.md`.

### Security

- PSK для TCP tunnel обязателен, не имеет встроенного default secret и может передаваться через `CHAMELEON_TUNNEL_PSK` без появления в process arguments;
- tunnel metadata аутентифицируется и шифруется AEAD;
- случайный KDF salt используется один раз на tunnel connection;
- replayed hello salt отклоняется в пределах clock-skew window;
- deploy хранит tunnel PSK вне репозитория в `/etc/chameleon/tunnel.env` с правами `0600`.

### Tests

- добавлены tests handshake/wrong PSK/expired timestamp;
- добавлен large-payload encrypted stream test;
- добавлен tunnel end-to-end test;
- добавлены SOCKS5 handshake/data tests;
- добавлен test автоматического обучения carrier success и DPI success на реальном response traffic;
- добавлен regression test: carrier-only success не должен преждевременно помечать DPI strategy успешной.

### Notes

- TCP tunnel пока является собственным encrypted carrier, а не TLS/HTTP2 masquerade;
- UDP/QUIC general-purpose proxy path и automatic application-level DPI probes остаются следующими этапами.

## [0.4.0] - 2026-09-25

### Added

- добавлен `pkg/networkctx` для best-effort определения физического сетевого контекста без root;
- виртуальные интерфейсы Tailscale/WireGuard/tun/tap отделены от физической identity сети;
- NetworkID сохраняется как короткий fingerprint без записи полного локального IP;
- добавлен `pkg/traffic` с классами web, interactive, streaming, bulk и realtime;
- добавлен `pkg/carrier` с локальной памятью маршрутов по `network + destination + traffic class + protocol`;
- carrier pool поддерживает direct, chameleon-udp, chameleon-tcp и внешний relay capability;
- добавлен `pkg/planner`, объединяющий Carrier Engine и DPI Strategy Engine в один план;
- DPI target автоматически меняется с конечного сайта на carrier endpoint, когда используется туннельный маршрут;
- добавлены failure scopes `carrier`, `dpi` и `both`, чтобы DPI-сбой не портил статистику рабочего маршрута;
- добавлена CLI `cmd/plan` для просмотра выбранного плана и записи измеренного результата;
- добавлен документ `docs/adaptive_planner.md`.

### Changed

- direct остаётся первым выбором без истории;
- игры/realtime сильнее штрафуют latency, streaming/bulk сильнее учитывают throughput;
- relay/DERP рассматривается как подключаемый fallback, а не обязательная основа протокола;
- при подтверждённом DPI-сбое planner может оставить direct carrier и эскалировать только DPI strategy.

### Tests

- добавлены тесты определения virtual/physical interface;
- добавлены тесты traffic classifier;
- добавлены тесты carrier compatibility, fallback и persistence;
- добавлены интеграционные тесты combined planner;
- добавлена regression-проверка: DPI failure не должен выталкивать рабочий direct carrier.

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
