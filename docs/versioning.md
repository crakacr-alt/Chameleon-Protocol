# Версионирование Chameleon

Chameleon использует понятную схему версий `MAJOR.MINOR.PATCH`.

## Что означает номер

- `PATCH` — исправление существующего поведения без большого нового слоя;
- `MINOR` — новый законченный компонент или режим с обратной совместимостью;
- `MAJOR` — новый пользовательский контракт или существенное изменение архитектуры.

Пример:

```text
0.9.0  QUIC stream carrier
0.9.1  UDP datagram path
1.0.0  first end-user client release
```

## Release flow

Каждая версия проходит одинаковую цепочку:

```text
release/vX.Y.Z
      ↓
tests + race + vet + build + deploy checks
      ↓
Pull Request
      ↓
main
      ↓
versions/vX.Y.Z
```

`main` содержит только последний слитый проверенный релиз.

`versions/vX.Y.Z` создаётся на merge commit и служит постоянной точкой,
которую можно checkout для воспроизводимой установки старой версии.

## Где хранится версия

Версия записывается одновременно в:

- `VERSION`;
- `pkg/version/version.go`;
- `CHANGELOG.md`.

CLI-программы поддерживают `--version`.

## Совместимость

До 1.0 wire protocol ещё может меняться между minor-релизами.

Начиная с 1.0 несовместимые изменения wire/config формата требуют либо
совместимого migration path, либо нового major version.

## Правило готовности

Версия не считается выпущенной только потому, что код написан.

До merge обязательны:

1. `gofmt`;
2. `go mod tidy` без diff;
3. `go vet ./...`;
4. `go test -race ./...`;
5. `go build ./...`;
6. shell syntax checks deployment scripts;
7. integration tests для нового transport/data path;
8. обновлённые README, CHANGELOG и профиль установки.

Если новый слой не реализован полностью, capability не рекламируется как
готовая. Например, наличие QUIC transport само по себе не означает поддержку
произвольного UDP proxying.
