# Chameleon Android

Android-клиент находится в каталоге `android/`, а минимальный bridge к общему
Go-core — в `mobile/`.

## Архитектура

Android UI не реализует Chameleon заново.

```text
Android UI / Foreground Service
            |
            v
       gomobile AAR
            |
            v
       mobile package
            |
            v
  pkg/clientapp + planner
            |
     +------+------+------+
     |             |      |
   direct         QUIC   TLS/TCP
```

Так Android, Windows и Linux используют один adaptive routing core и одну
валидацию конфигурации.

## Первый запуск

1. Установить APK.
2. Открыть Chameleon.
3. Нажать «Импортировать профиль».
4. Выбрать `client-profile.txt`, созданный на VPS:
   `/etc/chameleon/client-profile.txt`.
5. Нажать большую кнопку питания.

После запуска локальная точка входа:

```text
SOCKS5 127.0.0.1:1080
```

## Режимы

### Smart

`Smart — direct + Chameleon` работает как локальный SOCKS5 sidecar. Он не
занимает Android `VpnService`, поэтому может использоваться рядом с другим
активным VPN. Adaptive core может выбрать direct или Chameleon carrier.

### Proxy

`Proxy — только Chameleon` тоже остаётся локальным SOCKS5 sidecar, но direct
TCP carrier исключён. Этот режим нужен для принудительной проверки маршрута
через Chameleon.

### VPN — весь телефон

Начиная с Android `1.0.0-alpha.3` приложение умеет создавать настоящий
системный IPv4/IPv6 TUN через Android `VpnService`.

Путь:

```text
приложения Android
       |
       v
Android TUN
       |
       v
hev-socks5-tunnel 2.18.0
       |
       v
SOCKS5 127.0.0.1:1080
       |
       v
общий Chameleon Go core (Proxy)
       |
       v
QUIC / TLS / TCP -> Chameleon server
```

Chameleon исключает собственный package из своего TUN, чтобы upstream sockets
не попадали обратно в VPN и не создавали routing loop.

VPN mode использует Proxy core: трафик, уже захваченный системным TUN, не
получает direct fallback.

Android допускает только один независимый активный `VpnService`. Поэтому
другой системный VPN совместим с Chameleon **только в Smart sidecar mode**.

Приложение в целом поддерживает Android 6.0+ (API 23), но VPN/TUN mode в
alpha.3 требует Android 10+ (API 29), поскольку закреплённый upstream
hev-socks5-tunnel 2.18.0 собран с `APP_PLATFORM=android-29`.

Quick Settings tile, действие в уведомлении и виджет рабочего стола служат
быстрым включением/выключением Smart.

## Обновления

Приложение читает `android/releases/index.json` из public GitHub repository.

Индекс содержит Android version, protocol version, дату, minimum Android API,
официальный APK URL, SHA-256 и список изменений.

При включённой автоматической проверке приложение:

1. проверяет GitHub;
2. сравнивает semantic version;
3. скачивает новый APK через DownloadManager;
4. проверяет SHA-256;
5. показывает уведомление;
6. открывает системный Android installer после нажатия.

Android не разрешает обычному приложению тихо заменить себя без подтверждения
пользователя. Chameleon не пытается обходить это ограничение.

## Выбор старой версии

Экран «Версии и изменения» показывает весь индекс.

APK старой версии можно скачать, но Android обычно запрещает downgrade поверх
более нового `versionCode`. Для настоящего rollback может понадобиться удалить
текущую версию, а затем установить старую.

## Сборка

GitHub Actions выполняет:

```text
go test ./...
gomobile bind -> android/app/libs/chameleon.aar
Gradle assembleDebug / assembleRelease
```

Pull Request получает debug APK artifact.

Merge Android release в `main` дополнительно:

- собирает signed alpha APK;
- вычисляет SHA-256;
- создаёт/обновляет GitHub Release `android-vX.Y.Z`;
- прикладывает APK;
- записывает SHA-256 обратно в release index.

## Signing

Alpha использует signing key из закрытого GitHub Actions cache и предназначен
для тестирования ранних APK.

Перед stable Android 1.2 нужен постоянный signing key в GitHub Secrets. Его
private key нельзя хранить в открытом репозитории.

## Supported Android

Alpha.3:

```text
App minSdk 23 = Android 6.0+
VPN/TUN mode = Android 10+ / API 29
targetSdk 35
```

Фраза «все Android» технически недостижима: старые Android не имеют нужной
современной TLS/runtime/platform базы. API 23 выбран как практический широкий
минимум.

## Что ещё не заявляется готовым

- per-app routing;
- одновременный Chameleon VpnService + Tailscale VpnService;
- Android TV UI polish;
- Play Store distribution;
- stable release signing key.

Эти пункты остаются частью Android 1.2 roadmap.
