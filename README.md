# LunaHub

LunaHub is a personal self-hosted panel/installer for Ubuntu 24.04.

Current Step 01 goal:

- install and manage Xray-core for VLESS + REALITY + Vision;
- install and manage Hysteria2;
- create users from CLI;
- generate subscription links;
- keep configs reproducible from one local JSON database;
- run a temporary web/API service through systemd;
- provide recovery/debug commands.

This is the foundation stage. Do not treat it as a finished commercial panel yet.

## Default ports

- `443/tcp` — VLESS + REALITY + Vision via Xray
- `443/udp` — Hysteria2
- `80/tcp` — ACME HTTP challenge for Hysteria2 certificates
- `9443/tcp` — temporary LunaHub web/API service, HTTP at this stage

## Install from public GitHub repository

Use the safer two-step form. It avoids `/dev/fd` and makes errors easier to read.

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh -o /tmp/lunahub-install.sh
sudo env \
  LUNAHUB_REPO_URL=https://github.com/CodeNoSekal/lunahub.git \
  LUNAHUB_DOMAIN=lunahub.space \
  LUNAHUB_ACME_EMAIL=admin@lunahub.space \
  bash /tmp/lunahub-install.sh
```

If you use a real mailbox for ACME notifications, replace `admin@lunahub.space` with it.

## First commands after install

```bash
sudo lunahub doctor
sudo lunahub status
sudo lunahub user create --name "Test User" --email test@example.com
sudo lunahub apply
sudo lunahub user list
sudo lunahub sub show --email test@example.com
```

## Important warning

Keys and subscription links are credentials. Never publish generated files from:

- `/etc/lunahub/config.json`
- `/var/lib/lunahub/db.json`
- `/usr/local/etc/xray/config.json`
- `/etc/hysteria/config.yaml`
