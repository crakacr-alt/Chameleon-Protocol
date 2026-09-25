# Как выпускать версию

1. Выбрать SemVer-номер.
2. Обновить `VERSION`, README и `CHANGELOG.md`.
3. Если изменение заметное, добавить `docs/RELEASE_<version>.md`.
4. Запустить:

```bash
gofmt -w .
go vet ./...
go test -race ./...
go build ./...
```

5. Открыть отдельный PR.
6. Дождаться зелёных Go CI и CodeQL.
7. Слить PR в `main`.
8. Только после этого создать tag `vX.Y.Z`.

Tag запускает release workflow. Workflow ещё раз проверяет проект и создаёт
GitHub Release из проверенного commit.

Для этого исследовательского проекта release workflow публикует source release,
а не готовые сетевые бинарники.
