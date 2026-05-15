#!/usr/bin/env bash
set -Eeuo pipefail

echo "This removes LunaHub service and binary only. It does NOT remove Xray/Hysteria configs or user data by default."
read -r -p "Continue? [y/N] " answer
[[ "$answer" == "y" || "$answer" == "Y" ]] || exit 0

systemctl disable --now lunahub.service 2>/dev/null || true
rm -f /etc/systemd/system/lunahub.service
systemctl daemon-reload
rm -f /usr/local/bin/lunahub
rm -rf /opt/lunahub

echo "Removed LunaHub service and binary. Preserved: /etc/lunahub, /var/lib/lunahub, /var/log/lunahub."
