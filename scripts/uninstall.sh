#!/usr/bin/env bash
set -Eeuo pipefail

info() { echo -e "\033[1;34m[INFO]\033[0m $*"; }
ok() { echo -e "\033[1;32m[OK]\033[0m $*"; }
warn() { echo -e "\033[1;33m[WARN]\033[0m $*"; }

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo LUNAHUB_PURGE_CONFIRM=YES bash scripts/uninstall.sh" >&2
  exit 1
fi

if [[ "${LUNAHUB_PURGE_CONFIRM:-}" != "YES" ]]; then
  cat >&2 <<MSG
Refusing to purge without confirmation.

Run:
  sudo LUNAHUB_PURGE_CONFIRM=YES bash scripts/uninstall.sh

Or from GitHub:
  sudo LUNAHUB_PURGE_CONFIRM=YES bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh)
MSG
  exit 1
fi

info "Preserving SSH access in UFW"
ufw allow OpenSSH >/dev/null 2>&1 || true
ufw allow 22/tcp >/dev/null 2>&1 || true

info "Stopping services"
systemctl disable --now lunahub.service >/dev/null 2>&1 || true
systemctl disable --now xray.service >/dev/null 2>&1 || true
systemctl disable --now hysteria-server.service >/dev/null 2>&1 || true
systemctl disable --now hysteria-server@config.service >/dev/null 2>&1 || true

info "Removing systemd units and overrides"
rm -f /etc/systemd/system/lunahub.service
rm -f /etc/systemd/system/xray.service
rm -rf /etc/systemd/system/xray.service.d
rm -rf /etc/systemd/system/lunahub.service.d
rm -f /etc/systemd/system/hysteria-server.service
rm -f /etc/systemd/system/hysteria-server@.service
rm -rf /etc/systemd/system/hysteria-server.service.d
systemctl daemon-reload
systemctl reset-failed >/dev/null 2>&1 || true

info "Removing LunaHub files"
rm -f /usr/local/bin/lunahub
rm -rf /etc/lunahub
rm -rf /var/lib/lunahub
rm -rf /var/log/lunahub
rm -rf /var/backups/lunahub
rm -rf /opt/lunahub
rm -rf /root/lunahub-backup-*

info "Removing Xray files installed by this stack"
rm -f /usr/local/bin/xray
rm -f /usr/local/bin/geoip.dat
rm -f /usr/local/bin/geosite.dat
rm -rf /usr/local/etc/xray
rm -rf /usr/local/share/xray
rm -rf /var/log/xray
rm -rf /var/lib/xray
rm -rf /etc/xray

info "Removing Hysteria files installed by this stack"
rm -f /usr/local/bin/hysteria
rm -rf /etc/hysteria
rm -rf /var/lib/hysteria
rm -rf /var/log/hysteria
rm -rf /root/.cache/hysteria
rm -rf /root/.local/share/hysteria

info "Removing dedicated users"
userdel lunahub >/dev/null 2>&1 || true
userdel xray >/dev/null 2>&1 || true

info "Removing UFW rules opened by LunaHub"
ufw delete allow 80/tcp >/dev/null 2>&1 || true
ufw delete allow 443/tcp >/dev/null 2>&1 || true
ufw delete allow 443/udp >/dev/null 2>&1 || true
ufw delete allow 9443/tcp >/dev/null 2>&1 || true
ufw allow OpenSSH >/dev/null 2>&1 || true
ufw allow 22/tcp >/dev/null 2>&1 || true

ok "Purge complete"
cat <<'MSG'
Check leftovers:
  systemctl list-unit-files | grep -Ei 'lunahub|xray|hysteria' || true
  command -v lunahub || true
  command -v xray || true
  command -v hysteria || true
  find /etc /usr/local /var /root -maxdepth 4 \( -iname '*lunahub*' -o -iname '*xray*' -o -iname '*hysteria*' \) 2>/dev/null
MSG
