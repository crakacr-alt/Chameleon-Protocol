# Установка Chameleon на Ubuntu/Debian

Схема: **Android → промежуточный сервер (Ingress) → WireGuard → Exit → Интернет**.
Сайты видят IP Exit. Для одного сервера установите только Exit и импортируйте его прямой профиль.

## Установка из комплекта

Скопируйте `Chameleon-server-installer.tar.gz` на каждую VPS через WinSCP или scp:

```bash
mkdir -p /root/chameleon-installer
tar -xzf /root/Chameleon-server-installer.tar.gz -C /root/chameleon-installer
cd /root/chameleon-installer
sudo bash install.sh
```

Комплект содержит исходники и исправления. `install.sh` собирает именно эту версию,
не подменяя её кодом из GitHub. Для загрузки пакетов и Go нужен Интернет.
Не удаляйте каталог комплекта, если хотите повторно запускать его установщик.

Меню:

1. **Exit server** — установить/обновить выходной сервер.
2. **Intermediate server** — установить промежуточный сервер.
3. **Pair exit → intermediate server** — связать два сервера.
4. **Show status** — службы, WireGuard, адреса и пути к профилям.
5. **Exit** — выйти из установщика.

## Два сервера

1. На **Exit** выберите пункт **1**. Скопируйте публичный `Server 1 pairing key`
   и запишите порт Chameleon. Для новой установки мастер использует **9443**;
   при обновлении сохраняет существующий порт.
2. На **промежуточном сервере** выберите **2**, вставьте публичный ключ Exit.
   Обычно оставьте публичный порт **9443**, WireGuard **51821**.
   В поле `Chameleon port on Server 1` введите порт из первого шага.
   Скопируйте `Server 2 pairing key` и публичный IP этой VPS.
3. На **Exit** выберите **3**, введите IP и публичный ключ промежуточного сервера.
   Укажите те же публичный порт и порт WireGuard. Установщик проверяет handshake
   и создаёт клиентские профили.
4. На **Exit** выберите **4**, чтобы снова увидеть пути к профилям.

Нужны root/sudo, две разные VPS и свободные порты. В firewall панели хостинга
разрешите TCP+UDP на публичный порт промежуточного сервера и UDP на его порт
WireGuard. Exit должен иметь исходящий доступ к этой VPS и Интернету.
SSH-порт не меняется. Локальный UFW настраивается скриптами.

Схема соединяет **один промежуточный сервер с одним Exit**. Цепочки из трёх
и более серверов этим мастером не поддерживаются.

## Где взять конфигурацию клиента

Все профили находятся на **Exit**, в `/etc/chameleon`:

| Подключение | Рекомендуемый файл Auth v2 |
|---|---|
| Напрямую к Exit | `/etc/chameleon/client-profile-v2.txt` |
| Через промежуточный сервер | `/etc/chameleon/client-profile-relay-v2.txt` |

Relay-профиль появляется **после сопряжения**. Меню **Show status** печатает
пути и endpoint, не выводя секреты. Старые файлы без `-v2` сохраняются для
совместимости. На Android импортируйте текст нужного файла и выберите VPN.

Скачать файл через WinSCP либо:

```bash
scp root@EXIT_IP:/etc/chameleon/client-profile-relay-v2.txt ./client-profile.txt
```

Замените `EXIT_IP` адресом Exit. Передайте файл на телефон и импортируйте его
в приложении. Это секрет доступа: не публикуйте его. Между серверами копируются
только публичные pairing keys; приватные WireGuard-ключи остаются на своих VPS.

По умолчанию relay-профиль использует TLS. DNS работает через TLS-туннель,
если QUIC недоступен. Для QUIC включите его при сопряжении (UDP должен быть доступен):

```bash
sudo CHAMELEON_RELAY_QUIC=1 bash install.sh pair
```

## Проверка и обновление

```bash
sudo bash install.sh status
sudo chameleonctl health
sudo journalctl -u chameleon-tunnel -n 50 --no-pager
```

На клиенте проверьте открытие сайта и внешний IP: в режиме VPN он должен
совпадать с IP Exit. Только TLS handshake ещё не доказывает передачу трафика.

Для обновления из нового комплекта распакуйте его в отдельный каталог и
повторите установку Exit. `chameleonctl update` использует опубликованную
ветку GitHub; локальные исправления появятся там только после публикации.

Установка опубликованной версии напрямую из GitHub:

```bash
curl -fsSLo /root/chameleon-setup.sh \
  https://raw.githubusercontent.com/crakacr-alt/Chameleon-Protocol/main/deploy/setup-network.sh
sudo bash /root/chameleon-setup.sh
```

Эта команда устанавливает содержимое `main`, которое может отличаться от
выданного локального комплекта.

Неинтерактивные команды: `bash install.sh exit`, `ingress`, `pair`, `status`.
Параметры: `CHAMELEON_PORT`, `CHAMELEON_EXIT_PORT`, `CHAMELEON_RELAY_PORT`,
`CHAMELEON_WG_PORT`, `CHAMELEON_EXIT_PUBLIC_KEY`, `CHAMELEON_INGRESS_IP`,
`CHAMELEON_INGRESS_PUBLIC_KEY`.
