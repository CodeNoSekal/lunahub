#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT="lunahub"
DEFAULT_REPO_URL="https://github.com/CodeNoSekal/lunahub.git"
DEFAULT_REPO_BRANCH="main"

DOMAIN="${LUNAHUB_DOMAIN:-${LUNAHUB_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}' || echo '127.0.0.1')}}"
ACME_EMAIL="${LUNAHUB_ACME_EMAIL:-admin@${DOMAIN}}"
REPO_URL="${LUNAHUB_REPO_URL:-$DEFAULT_REPO_URL}"
REPO_BRANCH="${LUNAHUB_REPO_BRANCH:-$DEFAULT_REPO_BRANCH}"
PANEL_PORT="${LUNAHUB_PANEL_PORT:-9443}"
VPN_PORT="${LUNAHUB_VPN_PORT:-443}"

INSTALL_DIR="/opt/lunahub"
SRC_DIR="$INSTALL_DIR/src"
CONFIG_DIR="/etc/lunahub"
DATA_DIR="/var/lib/lunahub"
LOG_DIR="/var/log/lunahub"
BACKUP_DIR="/var/backups/lunahub"
XRAY_CONFIG="/usr/local/etc/xray/config.json"
HYSTERIA_CONFIG="/etc/hysteria/config.yaml"

info() { echo -e "\033[1;34m[INFO]\033[0m $*"; }
ok() { echo -e "\033[1;32m[OK]\033[0m $*"; }
warn() { echo -e "\033[1;33m[WARN]\033[0m $*"; }
fail() { echo -e "\033[1;31m[FAIL]\033[0m $*" >&2; exit 1; }

usage() {
  cat <<USAGE
LunaHub installer

Usage:
  bash install.sh [install|update|uninstall|doctor]

Environment:
  LUNAHUB_DOMAIN=example.com
  LUNAHUB_ACME_EMAIL=admin@example.com
  LUNAHUB_REPO_URL=https://github.com/CodeNoSekal/lunahub.git
  LUNAHUB_REPO_BRANCH=main
  LUNAHUB_PANEL_PORT=9443
  LUNAHUB_VPN_PORT=443

One-line install:
  sudo env LUNAHUB_DOMAIN=example.com LUNAHUB_ACME_EMAIL=admin@example.com \
    bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/install.sh)
USAGE
}

require_root() {
  [[ "${EUID}" -eq 0 ]] || fail "Run as root: sudo bash install.sh"
}

check_os() {
  [[ -f /etc/os-release ]] || fail "Cannot detect OS: /etc/os-release not found"
  # shellcheck disable=SC1091
  . /etc/os-release
  case "${ID:-}" in
    ubuntu|debian) ;;
    *) warn "Target OS is Ubuntu/Debian. Detected: ${ID:-unknown}. Continuing anyway." ;;
  esac
}

install_packages() {
  info "Installing base packages..."
  apt-get update -y
  DEBIAN_FRONTEND=noninteractive apt-get install -y \
    curl wget unzip jq openssl ca-certificates ufw git build-essential golang-go iproute2 dnsutils tar
}

ensure_users_and_dirs() {
  info "Creating users and directories..."

  if ! id lunahub >/dev/null 2>&1; then
    useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin lunahub
  fi
  if ! id xray >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin xray
  fi

  install -d -m 755 "$INSTALL_DIR" "$BACKUP_DIR" "$LOG_DIR"
  install -d -m 750 -o root -g lunahub "$CONFIG_DIR" "$DATA_DIR"
  install -d -m 750 -o root -g xray "$(dirname "$XRAY_CONFIG")"
  install -d -m 750 -o xray -g xray /var/log/xray
  install -d -m 755 "$(dirname "$HYSTERIA_CONFIG")"
}

backup_existing_config() {
  local stamp="$BACKUP_DIR/backup-$(date -u +%Y%m%d-%H%M%S)"
  install -d -m 700 "$stamp"

  [[ -f "$CONFIG_DIR/config.json" ]] && cp -a "$CONFIG_DIR/config.json" "$stamp/config.json"
  [[ -f "$DATA_DIR/db.json" ]] && cp -a "$DATA_DIR/db.json" "$stamp/db.json"
  [[ -f "$XRAY_CONFIG" ]] && cp -a "$XRAY_CONFIG" "$stamp/xray.config.json"
  [[ -f "$HYSTERIA_CONFIG" ]] && cp -a "$HYSTERIA_CONFIG" "$stamp/hysteria.config.yaml"

  ok "Backup directory: $stamp"
}

install_xray() {
  info "Installing/updating Xray-core..."
  bash -c "$(curl -fsSL https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
  command -v xray >/dev/null 2>&1 || fail "xray binary not found after installation"

  install -d -m 750 -o root -g xray "$(dirname "$XRAY_CONFIG")"
  install -d -m 750 -o xray -g xray /var/log/xray
}

install_hysteria() {
  info "Installing/updating Hysteria2..."
  local hy_installer="/tmp/hysteria-install.sh"
  curl -fsSL https://get.hy2.sh/ -o "$hy_installer"
  bash "$hy_installer"
  rm -f "$hy_installer"
  command -v hysteria >/dev/null 2>&1 || fail "hysteria binary not found after installation"
}

copy_sources() {
  info "Preparing project source in $SRC_DIR..."
  local current_dir
  current_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd 2>/dev/null || echo /tmp)"

  rm -rf "$SRC_DIR"

  if [[ -f "$current_dir/go.mod" && -d "$current_dir/cmd/lunahub" ]]; then
    mkdir -p "$SRC_DIR"
    tar -C "$current_dir" -cf - . | tar -C "$SRC_DIR" -xf -
  else
    git clone --depth 1 --branch "$REPO_BRANCH" "$REPO_URL" "$SRC_DIR"
  fi
}

build_binary() {
  info "Building /usr/local/bin/lunahub..."
  cd "$SRC_DIR"
  gofmt -w ./cmd/lunahub/main.go
  go build -trimpath -ldflags "-s -w" -o /usr/local/bin/lunahub ./cmd/lunahub
  chmod 755 /usr/local/bin/lunahub
}

parse_xray_keys() {
  local x25519="$1"
  PRIVATE_KEY="$(printf '%s\n' "$x25519" | awk -F': *' '
    tolower($1) == "privatekey" { print $2; exit }
    tolower($1) == "private key" { print $2; exit }
  ')"
  PUBLIC_KEY="$(printf '%s\n' "$x25519" | awk -F': *' '
    tolower($1) == "password (publickey)" { print $2; exit }
    tolower($1) == "publickey" { print $2; exit }
    tolower($1) == "public key" { print $2; exit }
  ')"
}

generate_config() {
  if [[ -f "$CONFIG_DIR/config.json" ]]; then
    ok "Config already exists: $CONFIG_DIR/config.json"
    return
  fi

  info "Generating LunaHub config..."
  local x25519 short_id obfs_password admin_token
  x25519="$(xray x25519)"
  parse_xray_keys "$x25519"

  if [[ -z "${PRIVATE_KEY:-}" || -z "${PUBLIC_KEY:-}" ]]; then
    printf '%s\n' "$x25519" >&2
    fail "Cannot parse Xray x25519 keys"
  fi

  short_id="$(openssl rand -hex 8)"
  obfs_password="$(openssl rand -base64 32 | tr -d '=+/ ' | cut -c1-24)"
  admin_token="$(openssl rand -hex 24)"

  cat > "$CONFIG_DIR/config.json" <<JSON
{
  "project": "LunaHub",
  "domain": "$DOMAIN",
  "acme_email": "$ACME_EMAIL",
  "admin_token": "$admin_token",
  "panel_listen": "0.0.0.0:$PANEL_PORT",
  "public_panel_url": "http://$DOMAIN:$PANEL_PORT",
  "paths": {
    "data_file": "$DATA_DIR/db.json",
    "xray_config": "$XRAY_CONFIG",
    "hysteria_config": "$HYSTERIA_CONFIG"
  },
  "xray": {
    "vless_port": $VPN_PORT,
    "reality_dest": "www.cloudflare.com:443",
    "reality_server_name": "www.cloudflare.com",
    "reality_private_key": "$PRIVATE_KEY",
    "reality_public_key": "$PUBLIC_KEY",
    "reality_short_id": "$short_id"
  },
  "hysteria": {
    "listen": ":$VPN_PORT",
    "obfs_password": "$obfs_password",
    "masquerade_url": "https://www.cloudflare.com/"
  }
}
JSON

  jq -e '.domain != "" and .admin_token != "" and .xray.reality_private_key != "" and .xray.reality_public_key != ""' "$CONFIG_DIR/config.json" >/dev/null
  chown root:lunahub "$CONFIG_DIR/config.json"
  chmod 640 "$CONFIG_DIR/config.json"
  ok "Generated: $CONFIG_DIR/config.json"
}

install_systemd() {
  info "Installing systemd units..."
  install -m 644 "$SRC_DIR/systemd/lunahub.service" /etc/systemd/system/lunahub.service

  install -d -m 755 /etc/systemd/system/xray.service.d
  cat > /etc/systemd/system/xray.service.d/20-lunahub-user.conf <<'UNIT'
[Service]
User=xray
Group=xray
UNIT

  systemctl daemon-reload
  systemctl enable lunahub.service
  systemctl enable xray.service || true
  systemctl enable hysteria-server.service || true
}

configure_firewall() {
  info "Configuring UFW..."
  ufw allow OpenSSH >/dev/null 2>&1 || true
  ufw allow 22/tcp >/dev/null 2>&1 || true
  ufw allow 80/tcp >/dev/null 2>&1 || true
  ufw allow "$VPN_PORT/tcp" >/dev/null 2>&1 || true
  ufw allow "$VPN_PORT/udp" >/dev/null 2>&1 || true
  ufw allow "$PANEL_PORT/tcp" >/dev/null 2>&1 || true
  ufw --force enable >/dev/null 2>&1 || true
  ok "Firewall rules applied"
}

init_runtime() {
  info "Initializing database and generated configs..."
  /usr/local/bin/lunahub init-db
  chown root:lunahub "$DATA_DIR/db.json"
  chmod 660 "$DATA_DIR/db.json"
  /usr/local/bin/lunahub apply --no-restart
}

start_services() {
  info "Starting services..."
  systemctl restart xray.service || warn "xray restart failed. Check: journalctl -u xray -n 100 --no-pager -l"
  systemctl restart hysteria-server.service || warn "hysteria restart failed. Check: journalctl -u hysteria-server -n 100 --no-pager -l"
  systemctl restart lunahub.service
}

print_summary() {
  local token panel
  token="$(jq -r '.admin_token' "$CONFIG_DIR/config.json")"
  panel="$(jq -r '.public_panel_url' "$CONFIG_DIR/config.json")/?token=$token"

  echo
  ok "LunaHub installed"
  echo "Domain: $DOMAIN"
  echo "Panel: $panel"
  echo "Health: $(jq -r '.public_panel_url' "$CONFIG_DIR/config.json")/health"
  echo
  echo "Useful commands:"
  echo "  sudo lunahub doctor"
  echo "  sudo lunahub user create --name \"Admin\" --email admin@example.com"
  echo "  sudo lunahub apply"
  echo "  sudo lunahub sub show --email admin@example.com"
  echo
  echo "Uninstall:"
  echo "  sudo LUNAHUB_PURGE_CONFIRM=YES bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh)"
}

run_install() {
  require_root
  check_os
  install_packages
  ensure_users_and_dirs
  backup_existing_config
  install_xray
  install_hysteria
  copy_sources
  build_binary
  generate_config
  install_systemd
  configure_firewall
  init_runtime
  start_services
  print_summary
}

run_update() {
  require_root
  check_os
  install_packages
  ensure_users_and_dirs
  backup_existing_config
  copy_sources
  build_binary
  install_systemd
  /usr/local/bin/lunahub apply --no-restart
  start_services
  ok "LunaHub updated"
}

run_doctor() {
  if command -v lunahub >/dev/null 2>&1; then
    lunahub doctor
  else
    fail "lunahub is not installed"
  fi
}

case "${1:-install}" in
  install) run_install ;;
  update) run_update ;;
  doctor) run_doctor ;;
  uninstall) exec bash <(curl -Ls https://raw.githubusercontent.com/CodeNoSekal/lunahub/main/scripts/uninstall.sh) ;;
  help|-h|--help) usage ;;
  *) usage; fail "Unknown command: ${1:-}" ;;
esac
