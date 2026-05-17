#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${LUNAHUB_PURGE_CONFIRM:-}" != "YES" ]]; then
  echo "Refusing to purge."
  echo "Run:"
  echo "  sudo LUNAHUB_PURGE_CONFIRM=YES bash scripts/uninstall.sh"
  echo "or:"
  echo "  curl -fsSL https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh | sudo LUNAHUB_PURGE_CONFIRM=YES bash -s"
  exit 1
fi

VPN_TCP_PORT="${LUNAHUB_VPN_TCP_PORT:-8443}"
VPN_UDP_PORT="${LUNAHUB_VPN_UDP_PORT:-443}"

echo "[1/10] Preserve SSH access in UFW if UFW is active"
ufw allow OpenSSH >/dev/null 2>&1 || true
ufw allow 22/tcp >/dev/null 2>&1 || true

echo "[2/10] Stop services"
systemctl disable --now lunahub.service >/dev/null 2>&1 || true
systemctl disable --now xray.service >/dev/null 2>&1 || true
systemctl disable --now hysteria-server.service >/dev/null 2>&1 || true
systemctl disable --now hysteria-server@config.service >/dev/null 2>&1 || true

echo "[3/10] Remove systemd units and overrides"
rm -f /etc/systemd/system/lunahub.service
rm -rf /etc/systemd/system/lunahub.service.d
rm -rf /etc/systemd/system/xray.service.d
rm -rf /etc/systemd/system/hysteria-server.service.d
systemctl daemon-reload
systemctl reset-failed >/dev/null 2>&1 || true

echo "[4/10] Remove LunaHub Caddy config"
if [[ -f /etc/caddy/Caddyfile ]]; then
  cp -f /etc/caddy/Caddyfile "/etc/caddy/Caddyfile.lunahub.bak.$(date +%Y%m%d-%H%M%S)" || true
  cat > /etc/caddy/Caddyfile <<'EOF_CADDY'
:80 {
  respond "Caddy is running"
}
EOF_CADDY
  caddy fmt --overwrite /etc/caddy/Caddyfile >/dev/null 2>&1 || true
  systemctl reload caddy.service >/dev/null 2>&1 || true
fi

echo "[5/10] Remove Let's Encrypt LunaHub certificate and deploy hook"
rm -f /etc/letsencrypt/renewal-hooks/deploy/lunahub-deploy-certs.sh
certbot delete --cert-name lunahub --non-interactive >/dev/null 2>&1 || true

echo "[6/10] Remove LunaHub files"
rm -f /usr/local/bin/lunahub
rm -rf /etc/lunahub
rm -rf /var/lib/lunahub
rm -rf /var/log/lunahub
rm -rf /var/backups/lunahub
rm -rf /opt/lunahub
rm -rf /var/www/lunahub-acme
rm -rf /root/lunahub-backup-*

echo "[7/10] Remove Xray files installed by this stack"
rm -f /usr/local/bin/xray
rm -f /usr/local/bin/geoip.dat
rm -f /usr/local/bin/geosite.dat
rm -rf /usr/local/etc/xray
rm -rf /usr/local/share/xray
rm -rf /var/log/xray
rm -rf /var/lib/xray
rm -rf /etc/xray

echo "[8/10] Remove Hysteria files installed by this stack"
rm -f /usr/local/bin/hysteria
rm -rf /etc/hysteria
rm -rf /var/lib/hysteria
rm -rf /var/log/hysteria
rm -rf /root/.cache/hysteria
rm -rf /root/.local/share/hysteria

echo "[9/10] Remove dedicated users"
userdel lunahub >/dev/null 2>&1 || true
userdel xray >/dev/null 2>&1 || true

echo "[10/10] Remove UFW rules opened by LunaHub"
ufw delete allow 80/tcp >/dev/null 2>&1 || true
ufw delete allow 443/tcp >/dev/null 2>&1 || true
ufw delete allow "$VPN_TCP_PORT/tcp" >/dev/null 2>&1 || true
ufw delete allow "$VPN_UDP_PORT/udp" >/dev/null 2>&1 || true

echo "Purge complete."
echo "Check leftovers with:"
echo "  systemctl list-unit-files | grep -Ei 'lunahub|xray|hysteria' || true"
echo "  command -v lunahub || true; command -v xray || true; command -v hysteria || true"
echo "  find /etc /usr/local /var /root -maxdepth 4 \\( -iname '*lunahub*' -o -iname '*xray*' -o -iname '*hysteria*' \\) 2>/dev/null"
