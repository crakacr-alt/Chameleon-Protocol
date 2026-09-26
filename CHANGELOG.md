# Changelog

Здесь записываются заметные изменения проекта по версиям.

## [1.0.0] - 2026-09-26

### Added

- единая пользовательская команда `chameleon`;
- `chameleon import` для профиля, который создаёт серверный installer;
- стабильный JSON config schema v1 с явным migration contract;
- режимы `smart` и `proxy`;
- единый client runtime поверх существующих adaptive planner, SOCKS5, QUIC/TLS/TCP и DPI engine;
- TCP bypass rules для exact host, domain suffix и IPv4/IPv6 CIDR;
- `chameleon doctor` с authenticated QUIC/TLS/TCP checks;
- `chameleon doctor --json` как безопасный диагностический экспорт без PSK;
- `chameleon show` с обязательным PSK redaction;
- `chameleon status` для локального SOCKS listener;
- Linux systemd client installer и hardened service;
- Windows PowerShell service installer;
- Windows amd64 cross-build в CI;
- PowerShell installer syntax validation в CI;
- документация `docs/client.md`.

### Changed

- пользовательский client entry point больше не требует вручную собирать десятки proxy flags;
- Smart mode сохраняет direct-first adaptive behavior;
- Proxy mode исключает direct TCP carrier из candidate set;
- proxy-mode UDP `auto` использует QUIC, если он настроен;
- config loader отказывается угадывать формат будущей неизвестной schema version.

### Tests

- profile import;
- config save/load и file-permission regression;
- future schema rejection;
- bypass matching;
- unified runtime Smart SOCKS5 end-to-end echo;
- Windows client cross-build;
- existing QUIC/TLS/TCP/UDP/race tests продолжают выполняться под race detector.

### Compatibility

- config schema v1 становится стабильным пользовательским контрактом;
- wire transport остаётся совместимым с 0.9.x server data paths;
- уже открытые application streams не обещают transparent migration при смене Wi-Fi/LTE — новые соединения используют новый network context;
- старые исследовательские CLI остаются в репозитории, но normal user flow теперь идёт через `chameleon`.

## [0.9.2] - 2026-09-26

### Added

- authenticated first-hop probes for raw TCP, TLS and QUIC carriers;
- `cmd/probe` with human-readable and JSON output;
- repeated samples with latency, jitter and loss summary;
- IPv4/IPv6 endpoint resolution and Happy-Eyeballs-style TCP racing;
- carrier memory now tracks observed jitter in addition to latency/throughput;
- dedicated probe destination handled inside the Chameleon server without opening Internet egress.

### Changed

- realtime/interactive carrier scoring now includes measured jitter penalty;
- network diagnostics prove the Chameleon PSK handshake instead of treating an open port as healthy;
- stale carrier evidence continues to decay, while new probe measurements refresh route knowledge.

### Tests

- address candidate resolution for IPv4/IPv6;
- Happy-Eyeballs winner selection;
- probe metrics and jitter calculation;
- authenticated probe path;
- jitter-aware carrier scoring regression tests.

### Notes

- probes are intentionally first-hop measurements; they do not claim that every final website is reachable;
- active probes complement real-session learning instead of replacing it;
- 1.0 consumes this diagnostics foundation for the unified client and doctor workflow.

## [0.9.1] - 2026-09-26

### Added

- SOCKS5 `UDP ASSOCIATE` в локальном proxy;
- настоящий QUIC DATAGRAM data path для application UDP;
- отдельный ALPN `chameleon-quic-dgram/1`, совместимый с stream-mode 0.9.0;
- PSK-authenticated control handshake до разрешения UDP relay;
- destination + payload datagram дополнительно защищаются Chameleon AEAD внутри QUIC TLS 1.3;
- direct UDP association без tunnel;
- client modes `--udp-mode=auto|direct|quic`;
- VPS UDP relay сохраняет packet boundaries и держит per-destination UDP sockets;
- лимит до 64 UDP destinations на одну association;
- явная `ErrDatagramTooLarge` вместо скрытой fragmentation;
- документация `docs/udp.md`.

### Changed

- `chameleon-quic` теперь честно объявляет `SupportsUDP=true`, потому что реальный executor появился;
- server QUIC listener одновременно обслуживает старый reliable-stream ALPN и новый datagram ALPN;
- SOCKS5 server теперь может обслуживать CONNECT и UDP ASSOCIATE через независимые transport interfaces.

### Tests

- SOCKS5 UDP ASSOCIATE round-trip;
- SOCKS UDP framing для IPv4, IPv6 и domain destinations;
- fragmented SOCKS UDP rejection;
- direct UDP echo test;
- QUIC DATAGRAM end-to-end через Chameleon server;
- wrong-PSK rejection;
- explicit datagram size-limit test.

### Notes

- 0.9.1 не фрагментирует большие application UDP payload;
- `auto` в этой версии выбирает QUIC UDP при наличии настроенного QUIC endpoint, иначе direct UDP;
- learned direct-vs-QUIC UDP probing выделен в 0.9.2.

## [0.9.0] - 2026-09-26

### Added

- настоящий QUIC carrier поверх UDP на базе `quic-go`;
- TLS 1.3 + обычная CA verification или SHA-256 certificate pinning для QUIC;
- Chameleon authenticated tunnel handshake и encrypted stream работают внутри QUIC bidirectional stream;
- сервер одновременно принимает TLS/TCP и QUIC/UDP на одном номере порта;
- новый adaptive carrier `chameleon-quic`;
- unknown-path race двух самых дешёвых carrier с небольшим stagger вместо последовательных длинных timeout;
- carrier memory получила `LastObservation` и time decay с half-life 24 часа;
- planner умеет определять наличие evidence для конкретной network/destination context;
- installer автоматически включает QUIC listener, открывает TCP+UDP firewall rules и сохраняет QUIC endpoint в client profile;
- health monitor проверяет наличие QUIC UDP listener;
- добавлены `VERSION`, `pkg/version` и `--version` для основных runtime binaries;
- добавлена документация `docs/quic.md` и `docs/versioning.md`.

### Changed

- toolchain проекта поднят до Go 1.27;
- `quic-go` закреплён на v0.63.0;
- stale carrier failures больше не влияют на выбор маршрута бесконечно;
- TCP-only DPI strategies не применяются к QUIC для видимости: QUIC использует direct DPI policy до появления реального UDP packet backend;
- CI теперь дополнительно проверяет, что `go mod tidy` не меняет `go.mod/go.sum`.

### Tests

- добавлен QUIC end-to-end tunnel test;
- добавлена проверка TLS certificate pinning для QUIC;
- добавлены тесты staggered carrier racing;
- добавлены тесты time decay и context-specific evidence;
- добавлен regression test, запрещающий назначать TCP split strategy на QUIC;
- полный `go test ./...` также проверен на отдельном Ubuntu host с Go 1.27.1.

### Notes

- 0.9.0 переносит TCP proxy streams через реальный UDP/QUIC transport;
- SOCKS5 UDP ASSOCIATE и arbitrary application UDP datagrams намеренно не рекламируются как готовые в 0.9.0;
- полноценный UDP datagram data path выделен в 0.9.1.

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
