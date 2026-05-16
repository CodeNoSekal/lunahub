# LunaHub

LunaHub — самостоятельная панель управления VPN-сервером в стиле 3x-ui, но со своей простой архитектурой. Проект ставит Xray-core с VLESS REALITY, Hysteria2, создаёт конфиги, управляет пользователями, отдаёт subscription-ссылки и поддерживает полную очистку сервера.

## Быстрая установка

```bash
sudo env \
  LUNAHUB_DOMAIN=example.com \
  LUNAHUB_ACME_EMAIL=admin@example.com \
  bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh)
```

Минимальный вариант:

```bash
sudo bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh)
```

Для Hysteria2/ACME лучше указывать реальный домен, который уже смотрит на IP сервера.

## Обновление

```bash
sudo bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh) update
```

## Полное удаление

```bash
sudo LUNAHUB_PURGE_CONFIRM=YES bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh)
```

Удаляются LunaHub, Xray, Hysteria2, systemd units, конфиги, база, runtime-файлы и UFW-правила, открытые установщиком. SSH-доступ сохраняется.

## Что устанавливается

| Компонент | Назначение | Порт |
|---|---|---:|
| LunaHub panel | веб-панель, CLI, subscription endpoint | `9443/tcp` |
| Xray-core | VLESS REALITY | `443/tcp` |
| Hysteria2 | UDP-протокол с obfs | `443/udp` |
| ACME HTTP challenge | выпуск сертификата Hysteria2 | `80/tcp` |

## Основные пути

```text
/usr/local/bin/lunahub
/etc/lunahub/config.json
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
/opt/lunahub/src
```

## Переменные установки

| Переменная | Пример | Назначение |
|---|---|---|
| `LUNAHUB_DOMAIN` | `example.com` | домен панели и VPN |
| `LUNAHUB_ACME_EMAIL` | `admin@example.com` | email для ACME |
| `LUNAHUB_REPO_URL` | `https://github.com/CodeNoSekal/lunahub.git` | репозиторий для установки |
| `LUNAHUB_REPO_BRANCH` | `main` | ветка установки |
| `LUNAHUB_PANEL_PORT` | `9443` | порт панели |
| `LUNAHUB_VPN_PORT` | `443` | порт Xray/Hysteria2 |

## Команды после установки

Проверить состояние:

```bash
sudo lunahub doctor
sudo lunahub status
curl -s http://127.0.0.1:9443/health
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

Пересоздать admin token:

```bash
sudo lunahub token rotate
sudo systemctl restart lunahub
```

## Веб-панель

После установки установщик покажет URL вида:

```text
http://DOMAIN:9443/?token=ADMIN_TOKEN
```

В панели можно:

- создавать пользователей;
- отключать пользователей;
- включать пользователей;
- удалять пользователей;
- пересоздавать ключи пользователя;
- применять конфиги вручную;
- смотреть состояние пользователей.

## Безопасность

Не публикуй содержимое этих файлов:

```text
/etc/lunahub/config.json
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
```

Там находятся `admin_token`, REALITY private key, UUID, Hysteria passwords, subscription tokens и obfs password.

Панель на первом этапе работает по HTTP на отдельном порту. Для нормальной публичной эксплуатации лучше закрыть `9443/tcp` firewall-ом и пустить панель через reverse proxy с TLS и дополнительной авторизацией.

## Разработка

```bash
go test ./...
go build -o /tmp/lunahub ./cmd/lunahub
bash -n install.sh
bash -n scripts/uninstall.sh
```

Или одной командой:

```bash
bash scripts/dev-check.sh
```

Подробные операции: [docs/OPERATIONS.md](docs/OPERATIONS.md).
