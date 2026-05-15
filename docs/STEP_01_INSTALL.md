# Step 01 — Installation and first check

## 1. Replace repository files

Download `lunahub_step01_fixed.zip`, unpack it, and replace the contents of your GitHub repository with the unpacked files.

Then push:

```bash
git add .
git commit -m "Fix Step 01 foundation files"
git push origin main
```

## 2. Install on server

SSH into the server:

```bash
ssh root@lunahub.space
```

Download installer explicitly:

```bash
curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh -o /tmp/lunahub-install.sh
```

Run installer:

```bash
sudo env \
  LUNAHUB_REPO_URL=https://github.com/CodeNoSekal/lunahub.git \
  LUNAHUB_DOMAIN=lunahub.space \
  LUNAHUB_ACME_EMAIL=admin@lunahub.space \
  bash /tmp/lunahub-install.sh
```

If you do not own `admin@lunahub.space`, use your real email instead. This email is only for ACME certificate notices.

## 3. Check services

```bash
sudo lunahub doctor
sudo lunahub status
sudo systemctl status lunahub --no-pager
sudo systemctl status xray --no-pager
sudo systemctl status hysteria-server --no-pager
```

## 4. Create first test user

```bash
sudo lunahub user create --name "Test User" --email test@example.com
sudo lunahub apply
sudo lunahub user list
sudo lunahub sub show --email test@example.com
```

## 5. Open temporary panel

The installer prints a URL like this:

```text
http://lunahub.space:9443/?token=ADMIN_TOKEN
```

This is a temporary Step 01 panel. Real login, users page, Telegram bot, email and updates come later.

## 6. If installation fails

Collect these outputs:

```bash
cat /etc/os-release
ls -la /root
ls -la /opt/lunahub/src
sudo journalctl -u lunahub -n 100 --no-pager
sudo journalctl -u xray -n 100 --no-pager
sudo journalctl -u hysteria-server -n 100 --no-pager
```
