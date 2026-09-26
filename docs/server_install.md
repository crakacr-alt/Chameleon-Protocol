# VPS installation: Chameleon 0.8

Цель этого режима — установить сервер один раз и дальше управлять им через systemd.

Поддерживаемая основная платформа: Ubuntu/Debian Linux.

## Самый простой вариант

Скопируйте репозиторий на сервер и выполните:

```bash
cd Chameleon-Protocol
sudo ./deploy/install-server.sh
```

Installer сам:

- проверит/установит базовые пакеты;
- установит Go 1.25, если подходящей версии нет;
- соберёт и протестирует Chameleon;
- создаст системного пользователя `chameleon`;
- создаст PSK, если он ещё не существует;
- создаст TLS certificate и key, если свои не переданы;
- установит server и proxy binaries в `/usr/local/bin`;
- поставит hardened systemd service;
- включит automatic restart;
- включит health-check каждые 5 минут;
- при активном UFW/firewalld откроет только выбранный TCP port;
- сохранит параметры клиента в root-only файле.

Повторный запуск installer используется как upgrade и сохраняет существующий PSK
и автоматически созданный certificate.

## Порт

На новой установке Chameleon пытается использовать TCP/443.

Если 443 уже занят nginx, Apache или другим сервисом, выбирается 9443.

Можно указать порт явно:

```bash
sudo CHAMELEON_PORT=9443 ./deploy/install-server.sh
```

Installer не останавливает и не перенастраивает существующий web server.

## TLS

Если сертификат не указан, installer создаёт self-signed ECDSA certificate.

Клиент доверяет ему не через публичный CA, а через SHA-256 certificate pin,
который installer выводит после установки.

Если уже есть нормальный certificate:

```bash
sudo \
  CHAMELEON_TLS_CERT=/etc/letsencrypt/live/example.com/fullchain.pem \
  CHAMELEON_TLS_KEY=/etc/letsencrypt/live/example.com/privkey.pem \
  ./deploy/install-server.sh
```

Исходные certificate files не используются сервисом напрямую: installer
копирует их в управляемый каталог `/etc/chameleon`.

## PSK

По умолчанию генерируется случайный 256-bit PSK.

Если нужно задать свой:

```bash
sudo CHAMELEON_TUNNEL_PSK='YOUR_LONG_RANDOM_SECRET' ./deploy/install-server.sh
```

PSK хранится в:

```text
/etc/chameleon/tunnel.env
```

Файл доступен root и группе системного Chameleon service.

Client profile с секретом хранится отдельно:

```text
/etc/chameleon/client-profile.txt
```

Посмотреть его:

```bash
sudo chameleonctl client
```

## Управление

```bash
chameleonctl status
sudo chameleonctl health
sudo chameleonctl restart
sudo chameleonctl logs
sudo chameleonctl client
```

Логи также доступны напрямую:

```bash
journalctl -u chameleon-tunnel -f
```

## Health monitor

`chameleon-health.timer` раз в 5 минут делает локальный TLS/HTTP probe.

Если probe не проходит:

1. health script перезапускает tunnel service;
2. ждёт запуска;
3. повторяет probe;
4. при повторной ошибке systemd сохраняет failed health-check в journal.

Проверить timer:

```bash
systemctl status chameleon-health.timer
systemctl list-timers chameleon-health.timer
```

## Firewall

Если UFW уже активен, installer добавляет allow-rule только для выбранного TCP port.

Если firewalld уже активен, добавляется соответствующий permanent port.

Чтобы installer вообще не менял firewall:

```bash
sudo CHAMELEON_SKIP_FIREWALL=1 ./deploy/install-server.sh
```

## Public host

Installer пытается определить публичный IPv4 для client profile.

Если сервер находится за NAT или нужен domain:

```bash
sudo CHAMELEON_PUBLIC_HOST=example.com ./deploy/install-server.sh
```

## Что остаётся на клиенте

Для произвольного интернет-трафика сервер сам по себе не может перехватить трафик
телефона или компьютера. На клиенте нужен один из входов в Chameleon:

- local SOCKS5 proxy;
- будущий embedded/mobile client;
- router/gateway integration.

На Linux текущий proxy запускается так:

```bash
export CHAMELEON_TUNNEL_PSK='...'
chameleon-proxy \
  --chameleon-tls=SERVER:443 \
  --tls-fingerprint=SHA256_PIN
```

После этого приложения используют SOCKS5 `127.0.0.1:1080`.

## Основные файлы сервера

```text
/usr/local/bin/chameleon-tunnel-server
/usr/local/bin/chameleon-proxy
/usr/local/bin/chameleonctl
/usr/local/lib/chameleon/healthcheck.sh

/etc/chameleon/tunnel.env
/etc/chameleon/tls.crt
/etc/chameleon/tls.key
/etc/chameleon/decoy.html
/etc/chameleon/client-profile.txt

/etc/systemd/system/chameleon-tunnel.service
/etc/systemd/system/chameleon-health.service
/etc/systemd/system/chameleon-health.timer
```

## Ограничения текущего релиза

Server side для TCP уже можно держать как постоянный systemd service.

Пока не завершены:

- general-purpose UDP/QUIC proxy data path;
- полноценный Android client;
- Windows service/client installer;
- session resume уже отправленных application data при смене carrier;
- независимый security audit.

Поэтому 0.8 делает серверную эксплуатацию простой и устойчивой, но не означает,
что протокол гарантированно проходит любой существующий и будущий вид сетевой
фильтрации.
