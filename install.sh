#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT="lunahub"
DEFAULT_REPO_URL="https://github.com/CodeNoSekal/lunahub.git"
DEFAULT_REPO_BRANCH="main"

PANEL_DOMAIN="${LUNAHUB_PANEL_DOMAIN:-${LUNAHUB_DOMAIN:-}}"
VPN_DOMAIN="${LUNAHUB_VPN_DOMAIN:-${LUNAHUB_DOMAIN:-}}"
ACME_EMAIL="${LUNAHUB_ACME_EMAIL:-}"
REPO_URL="${LUNAHUB_REPO_URL:-$DEFAULT_REPO_URL}"
REPO_BRANCH="${LUNAHUB_REPO_BRANCH:-$DEFAULT_REPO_BRANCH}"
PANEL_PORT="${LUNAHUB_PANEL_PORT:-9443}"
VPN_TCP_PORT="${LUNAHUB_VPN_TCP_PORT:-8443}"
VPN_UDP_PORT="${LUNAHUB_VPN_UDP_PORT:-443}"

INSTALL_DIR="/opt/lunahub"
SRC_DIR="$INSTALL_DIR/src"
CONFIG_DIR="/etc/lunahub"
TLS_DIR="$CONFIG_DIR/tls"
DATA_DIR="/var/lib/lunahub"
LOG_DIR="/var/log/lunahub"
BACKUP_DIR="/var/backups/lunahub"
ACME_WEBROOT="/var/www/lunahub-acme"

CONFIG_FILE="$CONFIG_DIR/config.json"
XRAY_CONFIG="/usr/local/etc/xray/config.json"
HYSTERIA_CONFIG="/etc/hysteria/config.yaml"
CADDYFILE="/etc/caddy/Caddyfile"
LE_CERT_NAME="lunahub"
LE_LIVE_DIR="/etc/letsencrypt/live/$LE_CERT_NAME"
TLS_FULLCHAIN="$TLS_DIR/lunahub-fullchain.pem"
TLS_PRIVKEY="$TLS_DIR/lunahub-privkey.pem"

info() { echo -e "\033[1;34m[INFO]\033[0m $*"; }
ok() { echo -e "\033[1;32m[OK]\033[0m $*"; }
warn() { echo -e "\033[1;33m[WARN]\033[0m $*"; }
fail() { echo -e "\033[1;31m[FAIL]\033[0m $*"; exit 1; }

require_root() {
  [[ "${EUID}" -eq 0 ]] || fail "Run as root: sudo bash install.sh"
}

check_os() {
  [[ -f /etc/os-release ]] || fail "Cannot detect OS: /etc/os-release was not found"
  # shellcheck disable=SC1091
  . /etc/os-release
  [[ "${ID}" == "ubuntu" || "${ID}" == "debian" ]] || fail "Ubuntu/Debian is required. Current OS: ${ID:-unknown}"
  if [[ "${ID}" == "ubuntu" && "${VERSION_ID:-}" != "24.04" ]]; then
    warn "Target OS is Ubuntu 24.04. Current version: ${VERSION_ID:-unknown}. Continuing anyway."
  fi
}

read_tty() {
  local prompt="$1" value
  if [[ -r /dev/tty ]]; then
    read -r -p "$prompt" value </dev/tty
    printf '%s' "$value"
  else
    printf ''
  fi
}

ask_required() {
  local var_name="$1" label="$2" current="${!var_name:-}" value
  if [[ -n "$current" ]]; then
    return
  fi
  value="$(read_tty "$label: ")"
  [[ -n "$value" ]] || fail "Missing value: $label. You can pass it through the $var_name environment variable."
  printf -v "$var_name" '%s' "$value"
}

normalize_domain() {
  echo "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's#^https?://##; s#/.*$##; s/:.*$//'
}

validate_port() {
  local name="$1" value="$2"
  [[ "$value" =~ ^[0-9]+$ ]] || fail "$name must be a number: $value"
  (( value >= 1 && value <= 65535 )) || fail "$name must be between 1 and 65535: $value"
}

collect_install_settings() {
  echo
  echo "LunaHub installer"
  echo "-----------------"
  ask_required PANEL_DOMAIN "Panel domain, for example panel.example.com"
  ask_required VPN_DOMAIN "VPN domain, for example vpn.example.com"
  ask_required ACME_EMAIL "Let's Encrypt email, for example admin@example.com"

  PANEL_DOMAIN="$(normalize_domain "$PANEL_DOMAIN")"
  VPN_DOMAIN="$(normalize_domain "$VPN_DOMAIN")"

  [[ "$PANEL_DOMAIN" =~ ^[a-z0-9._-]+$ ]] || fail "Invalid panel domain: $PANEL_DOMAIN"
  [[ "$VPN_DOMAIN" =~ ^[a-z0-9._-]+$ ]] || fail "Invalid VPN domain: $VPN_DOMAIN"
  [[ "$ACME_EMAIL" == *"@"* ]] || fail "Invalid email: $ACME_EMAIL"
  validate_port "LUNAHUB_PANEL_PORT" "$PANEL_PORT"
  validate_port "LUNAHUB_VPN_TCP_PORT" "$VPN_TCP_PORT"
  validate_port "LUNAHUB_VPN_UDP_PORT" "$VPN_UDP_PORT"

  if [[ "$PANEL_DOMAIN" == "$VPN_DOMAIN" ]]; then
    warn "The panel and VPN use the same domain. Separate domains are recommended, but this setup is supported."
  fi

  echo
  info "Panel: https://$PANEL_DOMAIN"
  info "VPN: $VPN_DOMAIN"
  info "ACME email: $ACME_EMAIL"
  info "VLESS REALITY TCP: $VPN_TCP_PORT"
  info "Hysteria2 UDP: $VPN_UDP_PORT"
}

install_packages() {
  info "Installing base packages..."
  apt-get update -y
  DEBIAN_FRONTEND=noninteractive apt-get install -y \
    curl wget unzip jq openssl ca-certificates ufw git build-essential golang-go iproute2 dnsutils debian-keyring debian-archive-keyring apt-transport-https gnupg lsb-release

  if ! command -v caddy >/dev/null 2>&1; then
    info "Installing Caddy..."
    if ! DEBIAN_FRONTEND=noninteractive apt-get install -y caddy; then
      warn "Caddy was not found in the default apt repositories. Adding the official Caddy repository."
      curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/gpg.key | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
      curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null
      apt-get update -y
      DEBIAN_FRONTEND=noninteractive apt-get install -y caddy
    fi
  fi

  command -v certbot >/dev/null 2>&1 || DEBIAN_FRONTEND=noninteractive apt-get install -y certbot
}

create_users_and_dirs() {
  info "Creating users and directories..."

  if ! id lunahub >/dev/null 2>&1; then
    useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin lunahub
  fi
  if ! id xray >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin xray
  fi

  install -d -m 755 "$INSTALL_DIR" "$SRC_DIR"
  install -d -m 750 -o root -g root "$CONFIG_DIR" "$DATA_DIR"
  install -d -m 755 "$LOG_DIR" "$BACKUP_DIR" "$ACME_WEBROOT"
  install -d -m 750 -o root -g root "$TLS_DIR"
  install -d -m 750 -o root -g xray "$(dirname "$XRAY_CONFIG")"
  install -d -m 750 -o xray -g xray /var/log/xray
  install -d -m 755 /etc/hysteria
}

install_xray() {
  info "Installing or updating Xray-core..."
  bash -c "$(curl -fsSL https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
  command -v xray >/dev/null 2>&1 || fail "xray was not found after installation"
  install -d -m 750 -o root -g xray "$(dirname "$XRAY_CONFIG")"
  install -d -m 750 -o xray -g xray /var/log/xray
}

install_hysteria() {
  info "Installing or updating Hysteria2..."
  local hy_installer="/tmp/hysteria-install.sh"
  curl -fsSL https://get.hy2.sh/ -o "$hy_installer"
  bash "$hy_installer"
  rm -f "$hy_installer"
  command -v hysteria >/dev/null 2>&1 || fail "hysteria was not found after installation"
}

copy_sources() {
  info "Copying project sources to $SRC_DIR..."
  local current_dir
  current_dir="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || pwd)"
  rm -rf "$SRC_DIR"

  if [[ -f "$current_dir/go.mod" && -d "$current_dir/cmd" ]]; then
    mkdir -p "$SRC_DIR"
    cp -a "$current_dir/." "$SRC_DIR/"
  else
    git clone --depth 1 --branch "$REPO_BRANCH" "$REPO_URL" "$SRC_DIR"
  fi
}

build_binary() {
  info "Building lunahub binary..."
  cd "$SRC_DIR"
  gofmt -w ./cmd/lunahub/main.go
  go build -o /usr/local/bin/lunahub ./cmd/lunahub
  chmod 755 /usr/local/bin/lunahub
}

write_temporary_caddyfile() {
  info "Starting temporary HTTP server for certificate issuance..."
  install -d -m 755 /etc/caddy "$ACME_WEBROOT"
  cat > "$CADDYFILE" <<EOF_CADDY
:80 {
  root * $ACME_WEBROOT
  file_server
}
EOF_CADDY
  caddy fmt --overwrite "$CADDYFILE" >/dev/null 2>&1 || true
  systemctl enable --now caddy.service
  systemctl reload caddy.service || systemctl restart caddy.service
}

obtain_certificate() {
  info "Obtaining Let's Encrypt certificate for $PANEL_DOMAIN and $VPN_DOMAIN..."
  local cert_args=(-d "$PANEL_DOMAIN")
  if [[ "$VPN_DOMAIN" != "$PANEL_DOMAIN" ]]; then
    cert_args+=(-d "$VPN_DOMAIN")
  fi
  certbot certonly --webroot \
    -w "$ACME_WEBROOT" \
    --cert-name "$LE_CERT_NAME" \
    "${cert_args[@]}" \
    --non-interactive \
    --agree-tos \
    --email "$ACME_EMAIL" \
    --keep-until-expiring
}

copy_certificates() {
  info "Copying certificates to $TLS_DIR..."
  [[ -f "$LE_LIVE_DIR/fullchain.pem" ]] || fail "Fullchain was not found: $LE_LIVE_DIR/fullchain.pem"
  [[ -f "$LE_LIVE_DIR/privkey.pem" ]] || fail "Private key was not found: $LE_LIVE_DIR/privkey.pem"

  install -d -m 750 -o root -g root "$CONFIG_DIR" "$TLS_DIR"
  cp -f "$LE_LIVE_DIR/fullchain.pem" "$TLS_FULLCHAIN"
  cp -f "$LE_LIVE_DIR/privkey.pem" "$TLS_PRIVKEY"

  if getent group caddy >/dev/null 2>&1; then
    chown root:caddy "$CONFIG_DIR" "$TLS_DIR" "$TLS_FULLCHAIN" "$TLS_PRIVKEY"
    chmod 750 "$CONFIG_DIR" "$TLS_DIR"
    chmod 640 "$TLS_FULLCHAIN" "$TLS_PRIVKEY"
  else
    chown root:root "$TLS_FULLCHAIN" "$TLS_PRIVKEY"
    chmod 644 "$TLS_FULLCHAIN"
    chmod 600 "$TLS_PRIVKEY"
  fi
}

install_cert_deploy_hook() {
  info "Installing certificate deploy hook..."
  install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy
  cat > /etc/letsencrypt/renewal-hooks/deploy/lunahub-deploy-certs.sh <<EOF_HOOK
#!/usr/bin/env bash
set -Eeuo pipefail
LE_LIVE_DIR="$LE_LIVE_DIR"
TLS_DIR="$TLS_DIR"
TLS_FULLCHAIN="$TLS_FULLCHAIN"
TLS_PRIVKEY="$TLS_PRIVKEY"
CONFIG_DIR="$CONFIG_DIR"
install -d -m 750 "\$TLS_DIR"
cp -f "\$LE_LIVE_DIR/fullchain.pem" "\$TLS_FULLCHAIN"
cp -f "\$LE_LIVE_DIR/privkey.pem" "\$TLS_PRIVKEY"
if getent group caddy >/dev/null 2>&1; then
  chown root:caddy "\$CONFIG_DIR" "\$TLS_DIR" "\$TLS_FULLCHAIN" "\$TLS_PRIVKEY"
  chmod 750 "\$CONFIG_DIR" "\$TLS_DIR"
  chmod 640 "\$TLS_FULLCHAIN" "\$TLS_PRIVKEY"
else
  chmod 644 "\$TLS_FULLCHAIN"
  chmod 600 "\$TLS_PRIVKEY"
fi
systemctl reload caddy.service >/dev/null 2>&1 || true
systemctl restart hysteria-server.service >/dev/null 2>&1 || true
EOF_HOOK
  chmod 755 /etc/letsencrypt/renewal-hooks/deploy/lunahub-deploy-certs.sh
}

write_final_caddyfile() {
  info "Configuring HTTPS reverse proxy for the panel..."
  local http_hosts="http://$PANEL_DOMAIN"
  if [[ "$VPN_DOMAIN" != "$PANEL_DOMAIN" ]]; then
    http_hosts="$http_hosts, http://$VPN_DOMAIN"
  fi

  cat > "$CADDYFILE" <<EOF_CADDY
$http_hosts {
  root * $ACME_WEBROOT
  file_server
}

https://$PANEL_DOMAIN {
  tls $TLS_FULLCHAIN $TLS_PRIVKEY
  encode gzip
  header {
    X-Frame-Options DENY
    X-Content-Type-Options nosniff
    Referrer-Policy no-referrer
  }
  reverse_proxy 127.0.0.1:$PANEL_PORT
}
EOF_CADDY
  caddy fmt --overwrite "$CADDYFILE" >/dev/null 2>&1 || true
  caddy validate --config "$CADDYFILE"
  systemctl reload caddy.service || systemctl restart caddy.service
}

generate_base_config() {
  if [[ -f "$CONFIG_FILE" ]]; then
    ok "Config already exists: $CONFIG_FILE"
    return
  fi

  info "Generating initial config.json..."
  local x25519 private_key public_key short_id obfs_password admin_token
  x25519="$(xray x25519)"

  private_key="$(printf '%s\n' "$x25519" | awk -F': *' '
    $1 == "PrivateKey" { print $2; exit }
    $1 == "Private key" { print $2; exit }
  ')"

  public_key="$(printf '%s\n' "$x25519" | awk -F': *' '
    $1 == "Password (PublicKey)" { print $2; exit }
    $1 == "PublicKey" { print $2; exit }
    $1 == "Public key" { print $2; exit }
  ')"

  if [[ -z "$private_key" || -z "$public_key" ]]; then
    printf '%s\n' "$x25519" >&2
    fail "Could not parse xray x25519 keys"
  fi

  short_id="$(openssl rand -hex 8)"
  obfs_password="$(openssl rand -base64 32 | tr -d '=+/ ' | cut -c1-24)"
  admin_token="$(openssl rand -hex 24)"

  cat > "$CONFIG_FILE" <<EOF_JSON
{
  "panel_domain": "$PANEL_DOMAIN",
  "vpn_domain": "$VPN_DOMAIN",
  "acme_email": "$ACME_EMAIL",
  "admin_token": "$admin_token",
  "panel_listen": "127.0.0.1:$PANEL_PORT",
  "public_base_url": "https://$PANEL_DOMAIN",
  "paths": {
    "data_file": "$DATA_DIR/db.json",
    "xray_config": "$XRAY_CONFIG",
    "hysteria_config": "$HYSTERIA_CONFIG"
  },
  "tls": {
    "fullchain": "$TLS_FULLCHAIN",
    "privkey": "$TLS_PRIVKEY"
  },
  "xray": {
    "vless_port": $VPN_TCP_PORT,
    "reality_dest": "www.cloudflare.com:443",
    "reality_server_name": "www.cloudflare.com",
    "reality_private_key": "$private_key",
    "reality_public_key": "$public_key",
    "reality_short_id": "$short_id"
  },
  "hysteria": {
    "listen": ":$VPN_UDP_PORT",
    "obfs_password": "$obfs_password",
    "masquerade_url": "https://www.cloudflare.com/",
    "cert_file": "$TLS_FULLCHAIN",
    "key_file": "$TLS_PRIVKEY"
  }
}
EOF_JSON

  jq -e '.panel_domain != "" and .vpn_domain != "" and .public_base_url != "" and .xray.reality_private_key != "" and .xray.reality_public_key != ""' "$CONFIG_FILE" >/dev/null || fail "Generated config.json is invalid"
  chown root:root "$CONFIG_FILE"
  chmod 600 "$CONFIG_FILE"
}

install_systemd() {
  info "Installing systemd units..."
  install -m 644 "$SRC_DIR/systemd/lunahub.service" /etc/systemd/system/lunahub.service

  install -d -m 755 /etc/systemd/system/xray.service.d
  cat > /etc/systemd/system/xray.service.d/20-lunahub-user.conf <<'EOF_XRAY'
[Service]
User=xray
Group=xray
EOF_XRAY

  systemctl daemon-reload
  systemctl enable lunahub.service
  systemctl enable xray.service || true
  systemctl enable hysteria-server.service || true
}

init_database_and_configs() {
  info "Initializing database and VPN configs..."
  lunahub init-db
  chown root:root "$DATA_DIR/db.json"
  chmod 600 "$DATA_DIR/db.json"
  lunahub apply
}

configure_firewall() {
  info "Configuring UFW..."
  ufw allow OpenSSH >/dev/null || true
  ufw allow 22/tcp >/dev/null || true
  ufw allow 80/tcp >/dev/null || true
  ufw allow 443/tcp >/dev/null || true
  ufw allow "$VPN_TCP_PORT/tcp" >/dev/null || true
  ufw allow "$VPN_UDP_PORT/udp" >/dev/null || true
  ufw --force enable >/dev/null || true
  ok "Firewall rules applied"
}

start_services() {
  info "Starting services..."
  systemctl restart caddy.service
  systemctl restart xray.service || warn "xray did not start. Logs: journalctl -u xray -n 100 --no-pager -l"
  systemctl restart hysteria-server.service || warn "hysteria-server did not start. Logs: journalctl -u hysteria-server -n 100 --no-pager -l"
  systemctl restart lunahub.service
}

print_summary() {
  local token
  token="$(jq -r '.admin_token' "$CONFIG_FILE")"

  echo
  ok "LunaHub installed"
  echo "Panel: https://$PANEL_DOMAIN/?token=$token"
  echo "Health: https://$PANEL_DOMAIN/health"
  echo "VPN domain: $VPN_DOMAIN"
  echo "VLESS REALITY: $VPN_DOMAIN:$VPN_TCP_PORT/tcp"
  echo "Hysteria2: $VPN_DOMAIN:$VPN_UDP_PORT/udp"
  echo "Config: $CONFIG_FILE"
  echo "Database: $DATA_DIR/db.json"
  echo
  echo "Commands:"
  echo "  sudo lunahub doctor"
  echo "  sudo lunahub user create --name \"Ivan\" --email ivan@example.com"
  echo "  sudo lunahub apply"
  echo "  sudo lunahub sub show --email ivan@example.com"
}

main() {
  require_root
  check_os
  collect_install_settings
  install_packages
  create_users_and_dirs
  install_xray
  install_hysteria
  copy_sources
  build_binary
  write_temporary_caddyfile
  obtain_certificate
  copy_certificates
  install_cert_deploy_hook
  write_final_caddyfile
  generate_base_config
  install_systemd
  configure_firewall
  init_database_and_configs
  start_services
  print_summary
}

main "$@"
