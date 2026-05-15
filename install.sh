#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT="lunahub"
DOMAIN="${LUNAHUB_DOMAIN:-lunahub.space}"
ACME_EMAIL="${LUNAHUB_ACME_EMAIL:-admin@lunahub.space}"
REPO_URL="${LUNAHUB_REPO_URL:-}"
INSTALL_DIR="/opt/lunahub"
CONFIG_DIR="/etc/lunahub"
DATA_DIR="/var/lib/lunahub"
LOG_DIR="/var/log/lunahub"
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

create_dirs() {
  info "Creating LunaHub directories..."
  install -d -m 755 "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
}

install_xray() {
  info "Installing/updating Xray-core with official XTLS installer..."
  bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
}

install_hysteria() {
  info "Installing/updating Hysteria2 with official installer..."
  bash <(curl -fsSL https://get.hy2.sh/)
}

copy_sources() {
  info "Copying repository files to $INSTALL_DIR..."
  local src_dir
  src_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  if [[ -n "$REPO_URL" ]]; then
    rm -rf "$INSTALL_DIR/src"
    git clone "$REPO_URL" "$INSTALL_DIR/src"
  else
    if [[ ! -f "$src_dir/go.mod" ]]; then
      fail "LUNAHUB_REPO_URL is required when running install.sh from a raw URL. Example: LUNAHUB_REPO_URL=https://github.com/YOUR_GITHUB_USERNAME/lunahub.git bash <(curl -fsSL https://raw.githubusercontent.com/YOUR_GITHUB_USERNAME/lunahub/main/install.sh)"
    fi
    rm -rf "$INSTALL_DIR/src"
    mkdir -p "$INSTALL_DIR/src"
    cp -a "$src_dir/." "$INSTALL_DIR/src/"
  fi
}

build_binary() {
  info "Building lunahub binary..."
  cd "$INSTALL_DIR/src"
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
  obfs_password="$(openssl rand -base64 24 | tr -d '=+/ ' | cut -c1-24)"
  admin_token="$(openssl rand -hex 24)"

  cat > "$CONFIG_DIR/config.json" <<JSON
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
    "masquerade_url": "https://news.ycombinator.com/"
  }
}
JSON
  chmod 600 "$CONFIG_DIR/config.json"
  ok "Generated config: $CONFIG_DIR/config.json"
  echo ""
  echo "Admin token for temporary API access: $admin_token"
  echo "Save it. You can read it later with: sudo jq -r .admin_token $CONFIG_DIR/config.json"
  echo ""
}

install_systemd() {
  info "Installing systemd unit..."
  cp "$INSTALL_DIR/src/systemd/lunahub.service" /etc/systemd/system/lunahub.service
  systemctl daemon-reload
  systemctl enable lunahub.service
}

configure_firewall() {
  info "Configuring UFW rules..."
  ufw allow 22/tcp || true
  ufw allow 80/tcp || true
  ufw allow 443/tcp || true
  ufw allow 443/udp || true
  ufw allow 9443/tcp || true
  ufw --force enable || true
}

apply_initial_configs() {
  info "Generating Xray and Hysteria2 configs..."
  lunahub init-db || true
  lunahub apply || true
}

restart_services() {
  info "Restarting services..."
  systemctl restart lunahub.service || true
  systemctl restart xray.service || true
  systemctl enable --now hysteria-server.service || true
}

print_summary() {
  ok "LunaHub base installation complete."
  echo ""
  echo "Domain:        $DOMAIN"
  echo "Panel:         http://$DOMAIN:9443"
  echo "Config:        $CONFIG_DIR/config.json"
  echo "Data:          $DATA_DIR/db.json"
  echo "Xray config:   $XRAY_CONFIG"
  echo "Hysteria conf: $HYSTERIA_CONFIG"
  echo ""
  echo "Next commands:"
  echo "  sudo lunahub doctor"
  echo "  sudo lunahub user create --name \"Test User\" --email test@example.com"
  echo "  sudo lunahub apply"
  echo "  sudo lunahub sub show --email test@example.com"
  echo ""
  echo "Service logs:"
  echo "  journalctl -u lunahub -e --no-pager"
  echo "  journalctl -u xray -e --no-pager"
  echo "  journalctl -u hysteria-server -e --no-pager"
}

main() {
  require_root
  check_os
  install_packages
  create_dirs
  install_xray
  install_hysteria
  copy_sources
  build_binary
  generate_base_config
  install_systemd
  configure_firewall
  apply_initial_configs
  restart_services
  print_summary
}

main "$@"
