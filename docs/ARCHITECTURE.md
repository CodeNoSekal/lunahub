# LunaHub Architecture

LunaHub is not a VPN protocol implementation. It is a control panel and installer that manages proven network cores.

## Components

```text
LunaHub CLI/API
  ├─ local JSON database, later SQLite/PostgreSQL
  ├─ Xray config generator
  ├─ Hysteria2 config generator
  ├─ subscription generator
  ├─ temporary web dashboard
  └─ future Telegram/email modules

Xray-core
  └─ VLESS + REALITY + Vision on 443/tcp

Hysteria2
  └─ QUIC/UDP profile on 443/udp
```

## Source of truth

The source of truth is LunaHub data:

```text
/etc/lunahub/config.json
/var/lib/lunahub/db.json
```

Generated files:

```text
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
```

Do not manually edit generated files unless debugging. If you run `lunahub apply`, manual changes will be overwritten.

## User model

Each person must have their own credentials:

- unique VLESS UUID;
- unique Hysteria2 username/password;
- unique subscription token.

No shared UUIDs. No shared Hysteria2 passwords.

## Current limitations

This is Stage 01. Known limitations:

- JSON file storage instead of SQLite;
- temporary HTTP dashboard without real admin login;
- no Telegram bot yet;
- no email yet;
- no traffic accounting UI yet;
- no multi-server support yet.

These limitations are intentional. The first target is a reliable installer and reproducible protocol config generation.
