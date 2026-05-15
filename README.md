# LunaHub

LunaHub is a personal self-hosted panel/installer for Ubuntu 24.04.

Initial goal:

- install and manage Xray-core for VLESS + REALITY + Vision;
- install and manage Hysteria2;
- create users;
- generate subscription links;
- keep configs reproducible from one local database;
- run as a systemd service;
- provide CLI commands for recovery and debugging.

This repository is at the first foundation stage. Do not treat it as a finished commercial panel yet.

## Default ports

- `443/tcp` — VLESS + REALITY + Vision via Xray
- `443/udp` — Hysteria2
- `80/tcp` — ACME HTTP challenge for Hysteria2 certificates
- `9443/tcp` — LunaHub web/API service, HTTP at this stage

## Quick install from GitHub

After you push these files to your GitHub repository, use the explicit install form below. The installer needs `LUNAHUB_REPO_URL` because a raw one-line installer cannot automatically see the whole repository.

```bash
sudo LUNAHUB_REPO_URL=https://github.com/CodeNoSekal/lunahub.git \
  LUNAHUB_DOMAIN=lunahub.space \
  LUNAHUB_ACME_EMAIL=admin@lunahub.space \
  bash <(curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh)
```

## First commands after install

```bash
sudo lunahub doctor
sudo lunahub user create --name "Test User" --email test@example.com
sudo lunahub apply
sudo lunahub user list
sudo lunahub sub show --email test@example.com
```

## Important warning

Keys and subscription links are credentials. Never publish generated files from `/etc/lunahub`, `/var/lib/lunahub`, `/usr/local/etc/xray/config.json`, or `/etc/hysteria/config.yaml`.
