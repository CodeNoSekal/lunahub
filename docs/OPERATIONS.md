# LunaHub operations

## Полный цикл тестовой переустановки

```bash
sudo LUNAHUB_PURGE_CONFIRM=YES bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh)

sudo env \
  LUNAHUB_DOMAIN=example.com \
  LUNAHUB_ACME_EMAIL=admin@example.com \
  bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh)

sudo lunahub doctor
sudo lunahub user create --name "Test" --email test@example.com
sudo lunahub apply
sudo lunahub sub show --email test@example.com
```

## Проверка портов

```bash
sudo ss -lntup | grep -E ':(80|443|9443)\b'
```

Ожидаемо:

```text
xray      tcp/443
hysteria  udp/443
lunahub   tcp/9443
```

## Проверка сервисов

```bash
sudo systemctl status lunahub.service --no-pager -l
sudo systemctl status xray.service --no-pager -l
sudo systemctl status hysteria-server.service --no-pager -l
```

## Логи

```bash
sudo journalctl -u lunahub -n 100 --no-pager -l
sudo journalctl -u xray -n 100 --no-pager -l
sudo journalctl -u hysteria-server -n 100 --no-pager -l
```

## DNS

```bash
dig +short A example.com
dig +short AAAA example.com
```

Для ACME домен должен указывать на сервер, а порт `80/tcp` должен быть доступен снаружи.

## Типовые проблемы

### Hysteria2 не получает сертификат

Проверь DNS, порт `80/tcp` и UFW:

```bash
sudo ufw status verbose
sudo ss -lntup | grep ':80'
```

### Xray падает из-за REALITY ключей

Проверь config:

```bash
sudo jq -r '
  "private_len=\((.xray.reality_private_key // "") | length)",
  "public_len=\((.xray.reality_public_key // "") | length)"
' /etc/lunahub/config.json
```

Если длина `0`, пересоздай конфиг или удали `/etc/lunahub/config.json` и запусти установку заново.

### Permission denied у Xray

```bash
sudo chown root:xray /usr/local/etc/xray
sudo chown root:xray /usr/local/etc/xray/config.json
sudo chmod 750 /usr/local/etc/xray
sudo chmod 640 /usr/local/etc/xray/config.json
sudo systemctl restart xray.service
```

## Резервные копии

Установщик сохраняет старые конфиги в:

```text
/var/backups/lunahub/backup-YYYYMMDD-HHMMSS
```

## Ручное редактирование config.json

После изменения `/etc/lunahub/config.json` выполни:

```bash
sudo lunahub apply
sudo systemctl restart lunahub
```

## Проверка перед push

```bash
bash scripts/dev-check.sh
```
