#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${LUNAHUB_PURGE_CONFIRM:-}" != "YES" ]]; then
  echo "Refusing to purge."
  echo "Run:"
  echo "  sudo LUNAHUB_PURGE_CONFIRM=YES bash scripts/uninstall.sh"
  exit 1
fi

echo "[1/8] Preserve SSH access in UFW if UFW is active"
ufw allow OpenSSH >/dev/null 2>&1 || true
ufw allow 22/tcp >/dev/null 2>&1 || true

echo "[2/8] Stop services"
systemctl disable --now lunahub.service >/dev/null 2>&1 || true
systemctl disable --now xray.service >/dev/null 2>&1 || true
systemctl disable --now hysteria-server.service >/dev/null 2>&1 || true
systemctl disable --now hysteria-server@config.service >/dev/null 2>&1 || true

echo "[3/8] Remove systemd units and overrides"
rm -f /etc/systemd/system/lunahub.service
rm -f /etc/systemd/system/hysteria-server.service
rm -f /etc/systemd/system/hysteria-server@.service
rm -f /etc/systemd/system/xray.service
rm -rf /etc/systemd/system/xray.service.d
rm -rf /etc/systemd/system/lunahub.service.d
rm -rf /etc/systemd/system/hysteria-server.service.d
systemctl daemon-reload
systemctl reset-failed >/dev/null 2>&1 || true

echo "[4/8] Remove LunaHub files"
rm -f /usr/local/bin/lunahub
rm -rf /etc/lunahub
rm -rf /var/lib/lunahub
rm -rf /var/log/lunahub
rm -rf /var/backups/lunahub
rm -rf /opt/lunahub
rm -rf /root/lunahub-backup-*

echo "[5/8] Remove Xray files installed by this stack"
rm -f /usr/local/bin/xray
rm -f /usr/local/bin/geoip.dat
rm -f /usr/local/bin/geosite.dat
rm -rf /usr/local/etc/xray
rm -rf /usr/local/share/xray
rm -rf /var/log/xray
rm -rf /var/lib/xray
rm -rf /etc/xray

echo "[6/8] Remove Hysteria files installed by this stack"
rm -f /usr/local/bin/hysteria
rm -rf /etc/hysteria
rm -rf /var/lib/hysteria
rm -rf /var/log/hysteria
rm -rf /root/.cache/hysteria
rm -rf /root/.local/share/hysteria

echo "[7/8] Remove dedicated users"
userdel lunahub >/dev/null 2>&1 || true
userdel xray >/dev/null 2>&1 || true

echo "[8/8] Optionally remove UFW rules opened by LunaHub"
ufw delete allow 80/tcp >/dev/null 2>&1 || true
ufw delete allow 443/tcp >/dev/null 2>&1 || true
ufw delete allow 443/udp >/dev/null 2>&1 || true
ufw delete allow 9443/tcp >/dev/null 2>&1 || true

echo "Purge complete."
echo "Check leftovers with:"
echo "  systemctl list-unit-files | grep -Ei 'lunahub|xray|hysteria' || true"
echo "  find /etc /usr/local /var /root -maxdepth 4 \\( -iname '*lunahub*' -o -iname '*xray*' -o -iname '*hysteria*' \\) 2>/dev/null"
