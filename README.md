# LunaHub

LunaHub — своя минимальная панель управления VPN-сервером в стиле 3x-ui, но без копирования 3x-ui. Проект ставит Xray-core, Hysteria2, Caddy, Certbot, генерирует конфиги, выдаёт HTTPS subscription-ссылки и даёт управлять пользователями через веб-панель и CLI.

## Главная схема

Рекомендуется использовать два DNS-имени:

```text
panel.example.com  -> IP сервера
vpn.example.com    -> IP сервера
```

Панель работает только через HTTPS:

```text
https://panel.example.com/?token=ADMIN_TOKEN
```

VPN использует отдельный домен:

```text
vpn.example.com
```

Порты по умолчанию:

| Сервис | Назначение | Порт |
|---|---|---:|
| Caddy | HTTPS панель | `443/tcp` |
| Caddy/Certbot | ACME HTTP challenge | `80/tcp` |
| Xray-core | VLESS REALITY | `8443/tcp` |
| Hysteria2 | UDP VPN | `443/udp` |
| LunaHub | внутренний HTTP backend | `127.0.0.1:9443` |

Почему VLESS не на `443/tcp`: этот порт занимает HTTPS-панель через Caddy. Hysteria2 спокойно работает на `443/udp`, потому что UDP и TCP — разные сокеты.

## Установка одной командой

Интерактивный запуск. Скрипт сам спросит:

1. домен панели;
2. домен VPN;
3. email для Let's Encrypt.

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh | sudo bash -s
```

Без вопросов, через переменные:

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh | sudo env \
  LUNAHUB_PANEL_DOMAIN=panel.example.com \
  LUNAHUB_VPN_DOMAIN=vpn.example.com \
  LUNAHUB_ACME_EMAIL=admin@example.com \
  bash -s
```

Если на твоём VPS не работает `bash <(curl ...)`, используй именно `curl ... | sudo bash -s` или скачай файл:

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh -o /tmp/lunahub-install.sh
sudo bash /tmp/lunahub-install.sh
```

## Полное удаление

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh | sudo LUNAHUB_PURGE_CONFIRM=YES bash -s
```

Удаляются LunaHub, Xray, Hysteria2, конфиги, база, сертификат `lunahub`, deploy hook, systemd unit и UFW-правила проекта. SSH-доступ скрипт старается сохранить.

## Что умеет панель

- показывает пользователей;
- создаёт пользователей;
- включает и отключает пользователей;
- удаляет пользователей;
- меняет ключи пользователя;
- автоматически применяет конфиги после изменений;
- показывает и копирует HTTPS subscription-ссылку;
- показывает и копирует прямые ссылки VLESS REALITY и Hysteria2.

Кнопка «Копировать» есть прямо рядом с subscription URL.

## CLI

Проверка:

```bash
sudo lunahub doctor
sudo lunahub status
```

Создать пользователя:

```bash
sudo lunahub user create --name "Ivan" --email ivan@example.com
sudo lunahub apply
```

Показать подписку:

```bash
sudo lunahub sub show --email ivan@example.com
```

Управление пользователями:

```bash
sudo lunahub user list
sudo lunahub user disable --email ivan@example.com
sudo lunahub user enable --email ivan@example.com
sudo lunahub user rotate --email ivan@example.com
sudo lunahub user delete --email ivan@example.com
sudo lunahub apply
```

Сменить admin token панели:

```bash
sudo lunahub token rotate
sudo systemctl restart lunahub
```

## Основные пути

```text
/usr/local/bin/lunahub
/etc/lunahub/config.json
/etc/lunahub/tls/lunahub-fullchain.pem
/etc/lunahub/tls/lunahub-privkey.pem
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
/opt/lunahub/src
/etc/caddy/Caddyfile
```

## Проверка после установки

```bash
sudo lunahub doctor
curl -s https://panel.example.com/health
sudo ss -lntup | grep -E ':(80|443|8443|9443)\b'
```

Ожидаемо:

```text
caddy      tcp/80
caddy      tcp/443
xray       tcp/8443
hysteria   udp/443
lunahub    tcp/127.0.0.1:9443
```

## Диагностика

Логи:

```bash
sudo journalctl -u lunahub -n 100 --no-pager -l
sudo journalctl -u xray -n 100 --no-pager -l
sudo journalctl -u hysteria-server -n 100 --no-pager -l
sudo journalctl -u caddy -n 100 --no-pager -l
```

DNS:

```bash
dig +short A panel.example.com
dig +short A vpn.example.com
```

Проверка сертификата:

```bash
sudo certbot certificates
sudo ls -l /etc/lunahub/tls
```

## Безопасность

Не публикуй содержимое этих файлов:

```text
/etc/lunahub/config.json
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
/etc/lunahub/tls/lunahub-privkey.pem
```

В них есть admin token, private key, UUID, пароли пользователей, subscription tokens и TLS private key.

## Разработка

```bash
bash scripts/dev-check.sh
```

Или отдельно:

```bash
gofmt -w ./cmd/lunahub/main.go
go test ./...
go build -o /tmp/lunahub ./cmd/lunahub
bash -n install.sh
bash -n scripts/uninstall.sh
```
