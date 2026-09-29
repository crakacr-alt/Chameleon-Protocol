# Автоматическая установка двух серверов Chameleon

Начиная с 1.0.4 для Ubuntu/Debian можно использовать один интерактивный установщик
для обеих VPS.

Скачать его:

```bash
curl -fsSLo /root/chameleon-setup.sh \
  https://raw.githubusercontent.com/crakacr-alt/Chameleon-Protocol/main/deploy/setup-network.sh
chmod 700 /root/chameleon-setup.sh
sudo bash /root/chameleon-setup.sh
```

Установщик показывает меню:

1. **Server 1** — устанавливает/обновляет Chameleon exit и создаёт WireGuard identity.
2. **Server 2** — поднимает публичный ingress и private WireGuard link.
3. **Pair Server 1 -> Server 2** — завершает сопряжение и создаёт relay client profile.
4. **Status** — показывает состояние Chameleon, WireGuard и путь к профилям.

## Безопасность ключей

Установщик никогда не просит переносить private WireGuard key между серверами.
Строка **pairing key** — это только public WireGuard key. Её можно копировать
между VPS.

Tunnel PSK остаётся в root-only профиле:

```text
/etc/chameleon/client-profile.txt
/etc/chameleon/client-profile-relay.txt
```

Не вставляйте содержимое этих файлов в чат или публичный issue.

## Порядок первого запуска

### Server 1

```bash
sudo bash /root/chameleon-setup.sh
```

Выберите:

```text
1) Server 1 - install/update Chameleon exit
```

Скопируйте напечатанный **Server 1 pairing key**.

### Server 2

Запустите тот же файл и выберите:

```text
2) Server 2 - install public ingress
```

Вставьте pairing key Server 1. Значения портов можно оставить по умолчанию:

```text
Public Chameleon port: 9443
Private WireGuard port: 51821
Chameleon port on Server 1: 9443
```

После установки скопируйте **Server 2 pairing key**.

### Завершение на Server 1

Снова запустите установщик и выберите:

```text
3) Pair Server 1 -> Server 2
```

Нужно ввести только публичный IP Server 2, его pairing key и при необходимости
изменить порты.

После успешного handshake будет создан файл:

```text
/etc/chameleon/client-profile-relay.txt
```

Именно его нужно импортировать в Android/desktop client, если клиент должен
подключаться через Server 2.

## Неинтерактивный режим

Для автоматизации доступны команды:

```bash
sudo bash setup-network.sh server1
sudo bash setup-network.sh server2
sudo bash setup-network.sh pair
sudo bash setup-network.sh status
```

Параметры также можно передать через environment:

```text
CHAMELEON_EXIT_PUBLIC_KEY
CHAMELEON_INGRESS_IP
CHAMELEON_INGRESS_PUBLIC_KEY
CHAMELEON_RELAY_PORT
CHAMELEON_WG_PORT
CHAMELEON_EXIT_PORT
```

Private keys в environment не передаются.
