# LunaHub Architecture

Step 01 uses a simple single-server architecture.

```text
User/Client
   |
   | subscription URL
   v
LunaHub temporary panel/API :9443
   |
   | generates configs from local database
   v
Xray-core            Hysteria2
443/tcp              443/udp
VLESS REALITY        HY2 + ACME + salamander obfs
```

The source of truth is the local database:

```text
/var/lib/lunahub/db.json
```

Runtime configuration is generated from this database:

```text
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
```

Generated secrets are stored in:

```text
/etc/lunahub/config.json
```

Do not publish these files.
