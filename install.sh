#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT="lunahub"
DOMAIN="${LUNAHUB_DOMAIN:-lunahub.space}"
ACME_EMAIL="${LUNAHUB_ACME_EMAIL:-admin@lunahub.space}"
REPO_URL="${LUNAHUB_REPO_URL:-}"
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
fail() { echo -e "\033[1;31m[FAIL]\033[0m $*"; exit 1; }

require_root() {
  [[ "${EUID}" -eq 0 ]] || fail "Run as root: sudo bash install.sh"
}

check_os() {
  [[ -f /etc/os-release ]] || fail "Cannot detect OS. /etc/os-release not found."
  # shellcheck disable=SC1091
  . /etc/os-release
  [[ "${ID}" == "ubuntu" ]] || fail "This installer targets Ubuntu 24.04. Detected: ${ID:-unknown}"
  if [[ "${VERSION_ID}" != "24.04" ]]; then
    warn "Expected Ubuntu 24.04, detected ${VERSION_ID}. Continuing, but this is not the target OS."
  fi
}

install_packages() {
  info "Updating apt index and installing base packages..."
  apt-get update -y
  DEBIAN_FRONTEND=noninteractive apt-get install -y \
    curl wget unzip jq openssl ca-certificates ufw git build-essential golang-go iproute2 dnsutils
}

create_user_and_dirs() {
  info "Creating LunaHub user and directories..."
  if ! id lunahub >/dev/null 2>&1; then
    useradd --system --home /var/lib/lunahub --shell /usr/sbin/nologin lunahub
  fi

  install -d -m 755 "$INSTALL_DIR" "$SRC_DIR"
  install -d -m 750 -o root -g lunahub "$CONFIG_DIR" "$DATA_DIR"
  install -d -m 755 "$LOG_DIR" "$BACKUP_DIR"
}

install_xray() {
  info "Installing/updating Xray-core with official XTLS installer..."
  bash -c "$(curl -fsSL https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
  command -v xray >/dev/null 2>&1 || fail "xray binary was not found after installation"
}

install_hysteria() {
  info "Installing/updating Hysteria2 with official installer..."
  bash <(curl -fsSL https://get.hy2.sh/)
  command -v hysteria >/dev/null 2>&1 || fail "hysteria binary was not found after installation"
}

copy_sources() {
  info "Copying repository files to $SRC_DIR..."
  local current_dir
  current_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  rm -rf "$SRC_DIR"

  if [[ -n "$REPO_URL" ]]; then
    git clone --depth 1 "$REPO_URL" "$SRC_DIR"
  else
    if [[ ! -f "$current_dir/go.mod" ]]; then
      fail "LUNAHUB_REPO_URL is required when running install.sh from a raw URL."
    fi
    mkdir -p "$SRC_DIR"
    cp -a "$current_dir/." "$SRC_DIR/"
  fi
}

build_binary() {
  info "Building lunahub binary..."
  cd "$SRC_DIR"
  gofmt -w ./cmd/lunahub/main.go
  go build -o /usr/local/bin/lunahub ./cmd/lunahub
  chmod 755 /usr/local/bin/lunahub
}

generate_base_config() {
  if [[ -f "$CONFIG_DIR/config.json" ]]; then
    ok "Config already exists: $CONFIG_DIR/config.json"
    return
  fi

  info "Generating initial LunaHub config..."
  local x25519 private_key public_key short_id obfs_password admin_token
  x25519="$(xray x25519)"
  private_key="$(echo "$x25519" | awk '/Private key:/ {print $3}')"
  public_key="$(echo "$x25519" | awk '/Public key:/ {print $3}')"
  short_id="$(openssl rand -hex 8)"
  obfs_password="$(openssl rand -base64 32 | tr -d '=+/ ' | cut -c1-24)"
  admin_token="$(openssl rand -hex 24)"

  cat > "$CONFIG_DIR/config.json" <<EOF
{
  "domain": "$DOMAIN",
  "acme_email": "$ACME_EMAIL",
  "admin_token": "$admin_token",
  "panel_listen": "0.0.0.0:9443",
  "paths": {
    "data_file": "$DATA_DIR/db.json",
    "xray_config": "$XRAY_CONFIG",
    "hysteria_config": "$HYSTERIA_CONFIG"
  },
  "xray": {
    "vless_port": 443,
    "reality_dest": "www.cloudflare.com:443",
    "reality_server_name": "www.cloudflare.com",
    "reality_private_key": "$private_key",
    "reality_public_key": "$public_key",
    "reality_short_id": "$short_id"
  },
  "hysteria": {
    "listen": ":443",
    "obfs_password": "$obfs_password",
    "masquerade_url": "https://www.cloudflare.com/"
  }
}
EOF
  chown root:lunahub "$CONFIG_DIR/config.json"
  chmod 640 "$CONFIG_DIR/config.json"
  ok "Generated config: $CONFIG_DIR/config.json"
}

install_systemd() {
  info "Installing systemd service..."
  install -m 644 "$SRC_DIR/systemd/lunahub.service" /etc/systemd/system/lunahub.service
  systemctl daemon-reload
  systemctl enable lunahub.service
}

init_database_and_configs() {
  info "Initializing database and VPN configs..."
  lunahub init-db
  chown root:lunahub "$DATA_DIR/db.json"
  chmod 660 "$DATA_DIR/db.json"
  lunahub apply
}

configure_firewall() {
  info "Configuring UFW firewall..."
  ufw allow OpenSSH >/dev/null || true
  ufw allow 80/tcp >/dev/null || true
  ufw allow 443/tcp >/dev/null || true
  ufw allow 443/udp >/dev/null || true
  ufw allow 9443/tcp >/dev/null || true
  ufw --force enable >/dev/null || true
  ok "Firewall rules applied"
}

start_services() {
  info "Starting services..."
  systemctl restart xray.service || warn "xray restart failed. Check: journalctl -u xray -n 100 --no-pager"
  systemctl restart hysteria-server.service || warn "hysteria-server restart failed. Check: journalctl -u hysteria-server -n 100 --no-pager"
  systemctl restart lunahub.service
  systemctl --no-pager --full status lunahub.service || true
}

print_summary() {
  local token
  token="$(jq -r '.admin_token' "$CONFIG_DIR/config.json")"
  echo
  ok "LunaHub Step 01 installed"
  echo "Domain:        $DOMAIN"
  echo "Panel:         http://$DOMAIN:9443/?token=$token"
  echo "Healthcheck:   http://$DOMAIN:9443/health"
  echo "Config:        $CONFIG_DIR/config.json"
  echo "Database:      $DATA_DIR/db.json"
  echo
  echo "Next commands:"
  echo "  sudo lunahub doctor"
  echo "  sudo lunahub user create --name \"Test User\" --email test@example.com"
  echo "  sudo lunahub apply"
  echo "  sudo lunahub sub show --email test@example.com"
}

main() {
  require_root
  check_os
  install_packages
  create_user_and_dirs
  install_xray
  install_hysteria
  copy_sources
  build_binary
  generate_base_config
  install_systemd
  init_database_and_configs
  configure_firewall
  start_services
  print_summary
}

main "$@"
