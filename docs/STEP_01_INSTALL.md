# STEP 01 — First LunaHub upload and installation

This document is the first practical step. Goal: upload the initial repository files, install LunaHub on a clean Ubuntu 24.04 server, create the first test user, and verify VLESS/Hysteria2 configs.

## 0. What you already have

You said:

- repository name: `lunahub`;
- domain: `lunahub.space`;
- the domain points to your server;
- server was cleaned.

That is enough for Step 01.

## 1. Upload files to GitHub

On your local computer, unpack the archive I sent and open the folder:

```bash
cd lunahub
```

Initialize git if it is not initialized yet:

```bash
git init
git branch -M main
```

Add your remote repository. Replace `YOUR_GITHUB_USERNAME` with your real GitHub username:

```bash
git remote add origin https://github.com/YOUR_GITHUB_USERNAME/lunahub.git
```

Commit and push:

```bash
git add .
git commit -m "Initial LunaHub foundation"
git push -u origin main
```

## 2. Connect to server

```bash
ssh root@lunahub.space
```

If you connect by IP, that is also fine:

```bash
ssh root@SERVER_IP
```

## 3. Run installer

Use explicit variables. Replace `YOUR_GITHUB_USERNAME`:

```bash
export LUNAHUB_REPO_URL="https://github.com/YOUR_GITHUB_USERNAME/lunahub.git"
export LUNAHUB_DOMAIN="lunahub.space"
export LUNAHUB_ACME_EMAIL="admin@lunahub.space"

bash <(curl -fsSL https://raw.githubusercontent.com/YOUR_GITHUB_USERNAME/lunahub/main/install.sh)
```

If your GitHub repository is private, this raw URL will not work without authentication. In that case, clone manually:

```bash
apt update && apt install -y git
cd /root
git clone https://github.com/YOUR_GITHUB_USERNAME/lunahub.git
cd lunahub
LUNAHUB_DOMAIN=lunahub.space LUNAHUB_ACME_EMAIL=admin@lunahub.space bash install.sh
```

## 4. What the installer does

The installer:

- checks that it is running as root;
- expects Ubuntu 24.04;
- installs base packages;
- installs Xray-core using the official XTLS install script;
- installs Hysteria2 using the official Hysteria script;
- builds the `lunahub` binary;
- creates `/etc/lunahub/config.json`;
- creates `/var/lib/lunahub/db.json`;
- generates REALITY keys;
- generates Hysteria2 obfuscation password;
- writes Xray and Hysteria2 configs;
- creates `lunahub.service`;
- opens ports with UFW.

## 5. Check status

```bash
sudo lunahub doctor
sudo lunahub status
```

Expected: it should show that Xray, Hysteria2 and LunaHub are installed/enabled.

Check services directly:

```bash
systemctl status lunahub --no-pager
systemctl status xray --no-pager
systemctl status hysteria-server --no-pager
```

Logs:

```bash
journalctl -u lunahub -e --no-pager
journalctl -u xray -e --no-pager
journalctl -u hysteria-server -e --no-pager
```

## 6. Create first test user

```bash
sudo lunahub user create --name "Test User" --email test@example.com
sudo lunahub apply
sudo lunahub user list
sudo lunahub sub show --email test@example.com
```

`lunahub apply` regenerates:

- `/usr/local/etc/xray/config.json`;
- `/etc/hysteria/config.yaml`.

Then it restarts Xray and Hysteria2.

## 7. Open temporary panel

In browser:

```text
http://lunahub.space:9443
```

At this stage the panel is a temporary MVP dashboard. It is not the final admin interface yet. Use CLI for real management.

## 8. Subscription URL

For a user, run:

```bash
sudo lunahub sub show --email test@example.com
```

You will get:

- subscription URL;
- direct VLESS link;
- direct Hysteria2 link.

The subscription endpoint returns a base64 v2ray-style subscription containing both links.

## 9. Firewall ports

The installer opens:

```text
22/tcp    SSH
80/tcp    ACME HTTP challenge for Hysteria2
443/tcp   VLESS REALITY via Xray
443/udp   Hysteria2
9443/tcp  LunaHub temporary panel
```

## 10. Important security notes

Do not publish these files:

```text
/etc/lunahub/config.json
/var/lib/lunahub/db.json
/usr/local/etc/xray/config.json
/etc/hysteria/config.yaml
```

They contain private keys, user credentials or subscription tokens.

For the first step, the LunaHub service runs as root because it needs to write Xray/Hysteria configs and restart services. Later we will split this into a safer model: web service as a low-privilege user and a small privileged local agent.

## 11. If something fails

Run:

```bash
sudo lunahub doctor
journalctl -u lunahub -e --no-pager
journalctl -u xray -e --no-pager
journalctl -u hysteria-server -e --no-pager
```

Common issues:

- domain does not point to this server;
- port 80 is blocked, so Hysteria2 cannot obtain ACME certificate;
- provider blocks UDP 443;
- GitHub raw URL is wrong;
- repository is private;
- installer was not run as root.

## 12. Next development step

After Step 01 works, Step 02 will add:

- normal web login;
- user creation from web panel;
- Telegram bot foundation;
- email SMTP settings;
- better health checks.
