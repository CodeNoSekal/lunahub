# LunaHub Operations

## Рекомендуемая DNS-схема

```text
panel.example.com A <server-ip>
vpn.example.com   A <server-ip>
```

Можно использовать один домен, но лучше разделить панель и VPN.

## Установка

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh | sudo bash -s
```

Скрипт спросит домен панели, домен VPN и email для Let's Encrypt.

## Неинтерактивная установка

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh | sudo env \
  LUNAHUB_PANEL_DOMAIN=panel.example.com \
  LUNAHUB_VPN_DOMAIN=vpn.example.com \
  LUNAHUB_ACME_EMAIL=admin@example.com \
  bash -s
```

Дополнительные переменные:

| Переменная | По умолчанию | Назначение |
|---|---:|---|
| `LUNAHUB_REPO_URL` | `https://github.com/CodeNoSekal/lunahub.git` | репозиторий для клонирования |
| `LUNAHUB_REPO_BRANCH` | `main` | ветка |
| `LUNAHUB_PANEL_PORT` | `9443` | внутренний порт панели |
| `LUNAHUB_VPN_TCP_PORT` | `8443` | TCP порт VLESS REALITY |
| `LUNAHUB_VPN_UDP_PORT` | `443` | UDP порт Hysteria2 |

## Обновление

```bash
cd /opt/lunahub/src
git pull
sudo go build -o /usr/local/bin/lunahub ./cmd/lunahub
sudo install -m 644 systemd/lunahub.service /etc/systemd/system/lunahub.service
sudo systemctl daemon-reload
sudo systemctl restart lunahub
sudo lunahub apply
```

## Сертификаты

Certbot получает один сертификат с двумя SAN:

```text
panel.example.com
vpn.example.com
```

После выпуска сертификат копируется в:

```text
/etc/lunahub/tls/lunahub-fullchain.pem
/etc/lunahub/tls/lunahub-privkey.pem
```

Deploy hook:

```text
/etc/letsencrypt/renewal-hooks/deploy/lunahub-deploy-certs.sh
```

Проверка:

```bash
sudo certbot renew --dry-run
sudo certbot certificates
```

## Проверка сервисов

```bash
sudo systemctl status caddy --no-pager -l
sudo systemctl status lunahub --no-pager -l
sudo systemctl status xray --no-pager -l
sudo systemctl status hysteria-server --no-pager -l
```

## Управление пользователями

```bash
sudo lunahub user create --name "Ivan" --email ivan@example.com
sudo lunahub apply
sudo lunahub sub show --email ivan@example.com
```

После действий из веб-панели `apply` выполняется автоматически.

## Полное удаление

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh | sudo LUNAHUB_PURGE_CONFIRM=YES bash -s
```

После удаления проверить остатки:

```bash
systemctl list-unit-files | grep -Ei 'lunahub|xray|hysteria' || true
command -v lunahub || true
command -v xray || true
command -v hysteria || true
find /etc /usr/local /var /root -maxdepth 4 \( -iname '*lunahub*' -o -iname '*xray*' -o -iname '*hysteria*' \) 2>/dev/null
```
