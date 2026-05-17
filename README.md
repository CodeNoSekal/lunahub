# LunaHub

LunaHub is a minimal self-hosted VPN control panel inspired by the 3x-ui workflow, but implemented as a separate lightweight Go project. It installs Xray-core, Hysteria2, Caddy and Certbot, generates runtime configs, serves HTTPS subscription links and provides user management through both the web panel and CLI.

## Recommended layout

Use two DNS names that point to the same server IP:

```text
panel.example.com  -> server IP
vpn.example.com    -> server IP
```

The panel is served through HTTPS:

```text
https://panel.example.com/?token=ADMIN_TOKEN
```

The VPN profiles use the VPN domain:

```text
vpn.example.com
```

Default ports:

| Service | Purpose | Port |
|---|---|---:|
| Caddy | HTTPS panel | `443/tcp` |
| Caddy/Certbot | ACME HTTP challenge | `80/tcp` |
| Xray-core | VLESS REALITY | `8443/tcp` |
| Hysteria2 | UDP VPN | `443/udp` |
| LunaHub | internal HTTP backend | `127.0.0.1:9443` |

VLESS is not placed on `443/tcp` by default because HTTPS for the panel uses that TCP port. Hysteria2 can still use `443/udp` because TCP and UDP are separate sockets.

## One-command installation

Interactive installation. The script asks for the panel domain, VPN domain and Let's Encrypt email:

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh | sudo bash -s
```

Non-interactive installation:

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh | sudo env \
  LUNAHUB_PANEL_DOMAIN=panel.example.com \
  LUNAHUB_VPN_DOMAIN=vpn.example.com \
  LUNAHUB_ACME_EMAIL=admin@example.com \
  bash -s
```

Optional port overrides:

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh | sudo env \
  LUNAHUB_PANEL_DOMAIN=panel.example.com \
  LUNAHUB_VPN_DOMAIN=vpn.example.com \
  LUNAHUB_ACME_EMAIL=admin@example.com \
  LUNAHUB_VPN_TCP_PORT=8443 \
  LUNAHUB_VPN_UDP_PORT=443 \
  bash -s
```

If process substitution is not available on the VPS, download the installer first:

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh -o /tmp/lunahub-install.sh
sudo bash /tmp/lunahub-install.sh
```

## Full uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh | sudo LUNAHUB_PURGE_CONFIRM=YES bash -s
```

The uninstall script removes LunaHub, Xray, Hysteria2, configs, database, the `lunahub` certificate, deploy hook, systemd unit and LunaHub firewall rules. It keeps SSH access rules whenever possible.

## Web panel features

- list users;
- create users;
- enable and disable users;
- delete users;
- rotate user credentials;
- apply generated configs automatically after user changes;
- show and copy HTTPS subscription URLs;
- show and copy direct VLESS REALITY and Hysteria2 links.

## CLI

Health check:

```bash
sudo lunahub doctor
sudo lunahub status
```

Create a user:

```bash
sudo lunahub user create --name "Ivan" --email ivan@example.com
sudo lunahub apply
```

Show a subscription:

```bash
sudo lunahub sub show --email ivan@example.com
```

Manage users:

```bash
sudo lunahub user list
sudo lunahub user disable --email ivan@example.com
sudo lunahub user enable --email ivan@example.com
sudo lunahub user rotate --email ivan@example.com
sudo lunahub user delete --email ivan@example.com
sudo lunahub apply
```

Rotate the panel admin token:

```bash
sudo lunahub token rotate
sudo systemctl restart lunahub
```

## Main paths

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

## Post-install check

```bash
sudo lunahub doctor
curl -s https://panel.example.com/health
sudo ss -lntup | grep -E ':(80|443|8443|9443)\b'
```

Expected listeners with default ports:

```text
caddy      tcp/80
caddy      tcp/443
xray       tcp/8443
hysteria   udp/443
lunahub    tcp/127.0.0.1:9443
```

## Diagnostics

Logs:

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

Certificate check:

```bash
sudo certbot certificates
sudo ls -l /etc/lunahub/tls
```

## Security

Do not publish the contents of these files:

```text
/etc/lunahub/config.json
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
/etc/lunahub/tls/lunahub-privkey.pem
```

They contain the admin token, private key, UUIDs, user passwords, subscription tokens and TLS private key.

## Development check

```bash
bash scripts/dev-check.sh
```

Or run checks separately:

```bash
gofmt -w ./cmd/lunahub/main.go
go test ./...
go build -o /tmp/lunahub ./cmd/lunahub
bash -n install.sh
bash -n scripts/uninstall.sh
```
