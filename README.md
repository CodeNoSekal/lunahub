# LunaHub

LunaHub — минимальная веб-панель и CLI для управления VPN-сервером на базе:

- Xray-core / VLESS REALITY
- Hysteria2
- systemd
- Ubuntu 24.04

Текущий этап проекта: базовый foundation/install stage. Панель пока работает по HTTP на отдельном порту `9443` и предназначена для первичной проверки, а не для публичной production-эксплуатации без дополнительного reverse proxy/TLS/auth hardening.

## Что устанавливается

После успешной установки сервер должен иметь:

| Компонент | Назначение | Порт |
|---|---:|---:|
| LunaHub panel | Веб-панель и subscription endpoint | `9443/tcp` |
| Xray VLESS REALITY | TCP VPN inbound | `443/tcp` |
| Hysteria2 | UDP VPN inbound | `443/udp` |
| ACME HTTP challenge | Получение сертификата Hysteria2 | `80/tcp` |

Основные пути:

```text
/usr/local/bin/lunahub
/etc/lunahub/config.json
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
/opt/lunahub/src
```

## Требования

Поддерживаемая целевая ОС:

```text
Ubuntu 24.04 LTS
```

Нужен root-доступ.

Перед установкой домен должен указывать на IP сервера.

Проверка DNS:

```bash
dig +short A lunahub.space
dig +short AAAA lunahub.space
```

A-запись должна указывать на IP сервера.

Проверка портов после установки:

```bash
sudo ss -lntup | grep -E ':(80|443|9443)\b'
```

Ожидаемо:

```text
xray      tcp/443
hysteria  udp/443
lunahub   tcp/9443
```

## Быстрая установка

Клонировать репозиторий:

```bash
cd /root
git clone https://github.com/CodeNoSekal/lunahub.git lunahub-dev
cd /root/lunahub-dev
```

Запустить установку:

```bash
sudo env \
  LUNAHUB_DOMAIN=lunahub.space \
  LUNAHUB_ACME_EMAIL=admin@lunahub.space \
  bash ./install.sh
```

Переменные:

| Переменная | Назначение | Пример |
|---|---|---|
| `LUNAHUB_DOMAIN` | домен панели/VPN | `lunahub.space` |
| `LUNAHUB_ACME_EMAIL` | email для ACME/Let's Encrypt | `admin@lunahub.space` |
| `LUNAHUB_REPO_URL` | опционально: URL репозитория для установки из raw script | `https://github.com/CodeNoSekal/lunahub.git` |

Если запускать `install.sh` из локальной папки репозитория, `LUNAHUB_REPO_URL` не нужен.

## Что делает install.sh

Установщик выполняет:

1. Проверяет root и Ubuntu.
2. Устанавливает системные пакеты.
3. Создаёт системных пользователей:
   - `lunahub`
   - `xray`
4. Устанавливает Xray.
5. Устанавливает Hysteria2.
6. Копирует исходники в `/opt/lunahub/src`.
7. Собирает бинарник `/usr/local/bin/lunahub`.
8. Генерирует `/etc/lunahub/config.json`.
9. Генерирует REALITY keys через `xray x25519`.
10. Проверяет, что ключи не пустые.
11. Устанавливает systemd service для LunaHub.
12. Добавляет systemd override для Xray:

```ini
[Service]
User=xray
Group=xray
```

13. Открывает UFW-порты:
   - `22/tcp`
   - `80/tcp`
   - `443/tcp`
   - `443/udp`
   - `9443/tcp`
14. Инициализирует базу `/var/lib/lunahub/db.json`.
15. Генерирует Xray и Hysteria конфиги.
16. Запускает сервисы.

## Проверка после установки

```bash
sudo lunahub doctor
```

```bash
sudo systemctl status xray.service --no-pager -l
sudo systemctl status hysteria-server.service --no-pager -l
sudo systemctl status lunahub.service --no-pager -l
```

```bash
curl -s http://127.0.0.1:9443/health
```

Ожидаемый ответ:

```json
{"ok":true}
```

Проверка портов:

```bash
sudo ss -lntup | grep -E ':(443|9443)\b'
```

Ожидаемо:

```text
udp   UNCONN  *:443   users:(("hysteria",...))
tcp   LISTEN  *:9443  users:(("lunahub",...))
tcp   LISTEN  *:443   users:(("xray",...))
```

## Создание пользователя

```bash
sudo lunahub user create --name "admin" --email admin@example.com
```

После создания пользователя применить конфиги:

```bash
sudo lunahub apply
```

Показать subscription для пользователя:

```bash
sudo lunahub sub show --email admin@example.com
```

Список пользователей:

```bash
sudo lunahub user list
```

Отключить пользователя:

```bash
sudo lunahub user disable --email admin@example.com
sudo lunahub apply
```

Включить пользователя:

```bash
sudo lunahub user enable --email admin@example.com
sudo lunahub apply
```

## Панель

Панель доступна по адресу:

```text
http://DOMAIN:9443/?token=ADMIN_TOKEN
```

Посмотреть статус:

```bash
sudo lunahub status
```

Чтобы не засветить token в логах/чатах:

```bash
sudo lunahub status | sed -E 's/token=[a-f0-9]+/token=REDACTED/'
```

Важно: `admin_token`, subscription-ссылки, VLESS UUID, Hysteria passwords и REALITY private key являются секретами. Не публикуй их в issue, README, чатах, скриншотах и логах.

## Повторное применение конфигурации

После изменения пользователей или конфига:

```bash
sudo lunahub apply
```

Команда должна:

1. Проверить `/etc/lunahub/config.json`.
2. Сгенерировать временный Xray config.
3. Проверить его через:

```bash
xray run -test -config /path/to/temp/config.json
```

4. Записать `/usr/local/etc/xray/config.json`.
5. Выставить права:

```text
/usr/local/etc/xray              root:xray 750
/usr/local/etc/xray/config.json  root:xray 640
```

6. Записать `/etc/hysteria/config.yaml` с правами `600`.
7. Перезапустить сервисы.

## Полное удаление

В репозитории есть destructive uninstall script:

```text
scripts/uninstall.sh
```

Он удаляет LunaHub, Xray, Hysteria, systemd units, конфиги, базы, generated secrets, runtime-файлы и LunaHub backup-директории.

Не запускай его на сервере, где Xray или Hysteria используются другими проектами.

Запуск:

```bash
cd /root/lunahub-dev
sudo LUNAHUB_PURGE_CONFIRM=YES bash scripts/uninstall.sh
```

Скрипт специально требует переменную:

```bash
LUNAHUB_PURGE_CONFIRM=YES
```

Без неё он откажется удалять файлы.

## Что удаляет uninstall.sh

Сервисы:

```text
lunahub.service
xray.service
hysteria-server.service
hysteria-server@config.service
```

Systemd files:

```text
/etc/systemd/system/lunahub.service
/etc/systemd/system/xray.service
/etc/systemd/system/xray.service.d
/etc/systemd/system/hysteria-server.service
/etc/systemd/system/hysteria-server@.service
/etc/systemd/system/lunahub.service.d
/etc/systemd/system/hysteria-server.service.d
```

LunaHub:

```text
/usr/local/bin/lunahub
/etc/lunahub
/var/lib/lunahub
/var/log/lunahub
/var/backups/lunahub
/opt/lunahub
/root/lunahub-backup-*
```

Xray:

```text
/usr/local/bin/xray
/usr/local/bin/geoip.dat
/usr/local/bin/geosite.dat
/usr/local/etc/xray
/usr/local/share/xray
/var/log/xray
/var/lib/xray
/etc/xray
```

Hysteria:

```text
/usr/local/bin/hysteria
/etc/hysteria
/var/lib/hysteria
/var/log/hysteria
/root/.cache/hysteria
/root/.local/share/hysteria
```

Пользователи:

```text
lunahub
xray
```

UFW rules:

```text
80/tcp
443/tcp
443/udp
9443/tcp
```

SSH-доступ скрипт старается сохранить:

```text
OpenSSH
22/tcp
```

## Проверка после удаления

```bash
systemctl list-unit-files | grep -Ei 'lunahub|xray|hysteria' || true
systemctl list-units --all | grep -Ei 'lunahub|xray|hysteria' || true
```

```bash
command -v lunahub || true
command -v xray || true
command -v hysteria || true
```

```bash
find /etc /usr/local /var /root -maxdepth 4 \
  \( -iname '*lunahub*' -o -iname '*xray*' -o -iname '*hysteria*' \) \
  2>/dev/null
```

Если команды ничего существенного не вернули — сервер очищен для новой установки.

## Полный цикл переустановки для теста

```bash
cd /root
rm -rf lunahub-dev
git clone https://github.com/CodeNoSekal/lunahub.git lunahub-dev
cd /root/lunahub-dev
```

Удалить старую установку:

```bash
sudo LUNAHUB_PURGE_CONFIRM=YES bash scripts/uninstall.sh
```

Установить заново:

```bash
sudo env \
  LUNAHUB_DOMAIN=lunahub.space \
  LUNAHUB_ACME_EMAIL=admin@lunahub.space \
  bash ./install.sh
```

Проверить:

```bash
sudo lunahub doctor
curl -s http://127.0.0.1:9443/health
sudo ss -lntup | grep -E ':(443|9443)\b'
```

Создать пользователя:

```bash
sudo lunahub user create --name "admin" --email admin@example.com
sudo lunahub apply
```

Проверить сервисы:

```bash
sudo systemctl status xray.service --no-pager -l
sudo systemctl status hysteria-server.service --no-pager -l
sudo systemctl status lunahub.service --no-pager -l
```

Ожидаемый итог:

```text
xray.service             active
hysteria-server.service  active
lunahub.service          active
```

## Диагностика

### Xray падает с empty privateKey

Ошибка:

```text
Failed to build REALITY config. > empty "privateKey"
```

Проверить config:

```bash
sudo jq -r '
  "reality_private_key_len=\((.xray.reality_private_key // "") | length)",
  "reality_public_key_len=\((.xray.reality_public_key // "") | length)"
' /etc/lunahub/config.json
```

Нормально:

```text
reality_private_key_len=43
reality_public_key_len=43
```

Если значения `0`, значит ключи не сгенерировались или не распарсились.

Проверить формат вывода Xray:

```bash
xray x25519
```

Поддерживаемые форматы:

```text
PrivateKey: ...
Password (PublicKey): ...
```

или:

```text
Private key: ...
Public key: ...
```

### Xray падает с permission denied

Ошибка:

```text
open /usr/local/etc/xray/config.json: permission denied
```

Проверить права:

```bash
sudo systemctl cat xray.service
sudo ls -ld /usr/local/etc/xray
sudo ls -l /usr/local/etc/xray/config.json
id xray
```

Ожидаемо:

```text
/usr/local/etc/xray              root:xray 750
/usr/local/etc/xray/config.json  root:xray 640
```

Быстрый ремонт:

```bash
sudo chown root:xray /usr/local/etc/xray
sudo chown root:xray /usr/local/etc/xray/config.json
sudo chmod 750 /usr/local/etc/xray
sudo chmod 640 /usr/local/etc/xray/config.json
sudo systemctl restart xray.service
```

### Hysteria падает из-за your.domain.net

Ошибка:

```text
DNS problem: query timed out looking up A for your.domain.net
```

Это значит, что `/etc/hysteria/config.yaml` остался старым или с плейсхолдером.

Проверить:

```bash
sudo grep -nE 'your.domain.net|domains|email|listen' /etc/hysteria/config.yaml
```

Перегенерировать:

```bash
sudo lunahub apply
sudo systemctl restart hysteria-server.service
```

Если `lunahub apply` падает на Xray, Hysteria config не будет обновлён. Сначала исправь Xray config.

### Hysteria ACME/DNS error для реального домена

Проверить DNS:

```bash
dig +short A lunahub.space
dig +short AAAA lunahub.space
```

Проверить, открыт ли порт `80/tcp`:

```bash
sudo ufw status verbose
sudo ss -lntup | grep ':80'
```

Hysteria использует ACME для сертификата. Домен должен резолвиться на сервер, а порт `80/tcp` должен быть доступен снаружи.

### Hysteria пишет no OCSP stapling

Предупреждение:

```text
no OCSP stapling for [domain]: no OCSP server specified in certificate
```

Это не фатальная ошибка. Если рядом есть:

```text
server up and running
```

значит Hysteria запущена.

### Проверить последние логи

```bash
sudo journalctl -u lunahub -n 100 --no-pager -l
sudo journalctl -u xray -n 100 --no-pager -l
sudo journalctl -u hysteria-server -n 100 --no-pager -l
```

## Безопасность

Файлы с секретами:

```text
/etc/lunahub/config.json
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
```

В них могут быть:

- admin token
- REALITY private key
- VLESS UUID
- Hysteria passwords
- subscription tokens
- obfs password

Не публикуй эти файлы.

Рекомендуемые права:

```text
/etc/lunahub/config.json          root:lunahub 640
/var/lib/lunahub/db.json          root:lunahub 660
/usr/local/etc/xray/config.json   root:xray 640
/etc/hysteria/config.yaml         root:root 600
```

Если случайно засветил admin token, перегенерируй его:

```bash
sudo python3 - <<'PY'
import json, pathlib, secrets

p = pathlib.Path("/etc/lunahub/config.json")
cfg = json.loads(p.read_text())
cfg["admin_token"] = secrets.token_hex(24)
p.write_text(json.dumps(cfg, indent=2, ensure_ascii=False) + "\n")
PY

sudo chown root:lunahub /etc/lunahub/config.json
sudo chmod 640 /etc/lunahub/config.json
sudo systemctl restart lunahub.service
```

Если засветил subscription links, перегенерируй пользовательские credentials:

```bash
sudo python3 - <<'PY'
import json, pathlib, secrets, uuid, datetime

p = pathlib.Path("/var/lib/lunahub/db.json")
db = json.loads(p.read_text())
now = datetime.datetime.now(datetime.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z")

for u in db.get("users", []):
    u["vless_uuid"] = str(uuid.uuid4())
    u["hysteria_password"] = secrets.token_urlsafe(24).rstrip("=")
    u["subscription_token"] = secrets.token_urlsafe(32).rstrip("=")
    u["updated_at"] = now

p.write_text(json.dumps(db, indent=2, ensure_ascii=False) + "\n")
PY

sudo chown root:lunahub /var/lib/lunahub/db.json
sudo chmod 660 /var/lib/lunahub/db.json
sudo lunahub apply
```

## Команды разработки

Сборка:

```bash
go build -o /tmp/lunahub-test ./cmd/lunahub
```

Форматирование:

```bash
gofmt -w ./cmd/lunahub/main.go
```

Локальная проверка Go-пакетов:

```bash
go test ./...
```

Посмотреть изменения перед commit:

```bash
git diff
```

Commit:

```bash
git add install.sh scripts/uninstall.sh cmd/lunahub/main.go README.md
git commit -m "Fix installer and document clean reinstall flow"
git push
```

## Минимальный smoke-test для релиза

```bash
sudo LUNAHUB_PURGE_CONFIRM=YES bash scripts/uninstall.sh

sudo env \
  LUNAHUB_DOMAIN=lunahub.space \
  LUNAHUB_ACME_EMAIL=admin@lunahub.space \
  bash ./install.sh

sudo lunahub doctor
sudo lunahub user create --name "admin" --email admin@example.com
sudo lunahub apply

curl -s http://127.0.0.1:9443/health
sudo ss -lntup | grep -E ':(443|9443)\b'

sudo systemctl is-active xray.service
sudo systemctl is-active hysteria-server.service
sudo systemctl is-active lunahub.service
```

Ожидаемо:

```text
{"ok":true}
active
active
active
```
