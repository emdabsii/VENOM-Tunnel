#!/bin/bash
# ==============================================================================
#  VENOM TUNNEL v2.0 — One-Line Installer
#  Installs Go panel, Rust netprobe, C sysopt, Backhaul v0.7.2 & Venom CLI
#  Auto-generates random 5-digit port, secret path & strong credentials
# ==============================================================================
set -euo pipefail

REPO="${VENOM_REPO:-emdabsii/Venom-Tunnel}"
VER="${VENOM_VER:-v2.0.0}"
INSTALL_DIR="/opt/venom"

G=$'\e[38;5;46m'; C=$'\e[38;5;51m'; Y=$'\e[38;5;226m'; R=$'\e[38;5;196m'; W=$'\e[1;37m'; N=$'\e[0m'

[ "$(id -u)" -eq 0 ] || { echo -e "${R}[!] Please run as root (sudo bash install.sh)${N}"; exit 1; }

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo -e "${R}[!] Unsupported architecture: $ARCH${N}"; exit 1 ;;
esac

printf "\n%s  Installing VENOM TUNNEL %s (%s)...%s\n\n" "$G" "$VER" "$ARCH" "$N"

apt-get update -qq >/dev/null 2>&1 || true
apt-get install -y -qq curl wget tar openssl sqlite3 jq >/dev/null 2>&1 || true

mkdir -p "$INSTALL_DIR/sql" /etc/venom/tunnels /etc/venom/certs /etc/backhaul

# 1. Install Backhaul v0.7.2 if missing
if [ ! -x /etc/backhaul/backhaul ] && [ ! -x /usr/local/bin/backhaul ]; then
  printf "  %s[*]%s Installing Backhaul v0.7.2 engine...\n" "$C" "$N"
  wget -qO /tmp/bh.tar.gz "https://github.com/Musixal/Backhaul/releases/download/v0.7.2/backhaul_linux_${ARCH}.tar.gz"
  tar -xzf /tmp/bh.tar.gz -C /etc/backhaul && rm -f /tmp/bh.tar.gz
  chmod +x /etc/backhaul/backhaul
  ln -sf /etc/backhaul/backhaul /usr/local/bin/backhaul
fi

# 2. Download prebuilt Venom polyglot binaries from GitHub Releases (or build locally if inside repo)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/Makefile" ] && [ -f "$SCRIPT_DIR/backend/main.go" ]; then
  printf "  %s[*]%s Building Venom components from local source...\n" "$C" "$N"
  cp -r "$SCRIPT_DIR/sql/"* "$INSTALL_DIR/sql/"
  cp "$SCRIPT_DIR/cli/venom" /usr/local/bin/venom
  [ -x "$SCRIPT_DIR/dist/venomd" ] && cp "$SCRIPT_DIR/dist/venomd" /usr/local/bin/venomd
  [ -x "$SCRIPT_DIR/dist/venom-netprobe" ] && cp "$SCRIPT_DIR/dist/venom-netprobe" /usr/local/bin/venom-netprobe
  [ -x "$SCRIPT_DIR/dist/venom-sysopt" ] && cp "$SCRIPT_DIR/dist/venom-sysopt" /usr/local/bin/venom-sysopt
  if [ ! -x /usr/local/bin/venom-sysopt ] && command -v gcc >/dev/null 2>&1; then
    gcc -O2 -s "$SCRIPT_DIR/sysopt/sysopt.c" -o /usr/local/bin/venom-sysopt
  fi
  if [ ! -x /usr/local/bin/venomd ] && command -v go >/dev/null 2>&1; then
    (cd "$SCRIPT_DIR/backend" && go build -ldflags="-s -w" -o /usr/local/bin/venomd .)
  fi
  if [ ! -x /usr/local/bin/venom-netprobe ] && command -v cargo >/dev/null 2>&1; then
    (cd "$SCRIPT_DIR/netprobe" && cargo build --release && cp target/release/venom-netprobe /usr/local/bin/venom-netprobe)
  fi
else
  BASE_URL="https://github.com/${REPO}/releases/download/${VER}"
  printf "  %s[*]%s Downloading Venom release binaries (%s)...\n" "$C" "$N" "$ARCH"
  wget -qO /usr/local/bin/venomd "${BASE_URL}/venomd-linux-${ARCH}"
  wget -qO /usr/local/bin/venom-netprobe "${BASE_URL}/venom-netprobe-linux-${ARCH}"
  wget -qO /usr/local/bin/venom-sysopt "${BASE_URL}/venom-sysopt-linux-${ARCH}"
  wget -qO /usr/local/bin/venom "https://raw.githubusercontent.com/${REPO}/main/cli/venom"
  wget -qO "$INSTALL_DIR/sql/schema.sql" "https://raw.githubusercontent.com/${REPO}/main/sql/schema.sql"
fi

chmod +x /usr/local/bin/venom /usr/local/bin/venomd /usr/local/bin/venom-netprobe /usr/local/bin/venom-sysopt 2>/dev/null || true

# 3. Initialize random 5-digit port, secret path & credentials
INFO=$(/usr/local/bin/venomd init-info)
URL=$(echo "$INFO" | awk -F= '/^URL=/{print $2}')
USER=$(echo "$INFO" | awk -F= '/^USER=/{print $2}')
PASS=$(echo "$INFO" | awk -F= '/^PASS=/{print $2}')
PORT=$(echo "$INFO" | awk -F= '/^PORT=/{print $2}')

# Open firewall port if ufw is active
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q active; then
  ufw allow "${PORT}/tcp" >/dev/null 2>&1 || true
fi

# 4. Install systemd service for Venom Web Panel
cat > /etc/systemd/system/venom-panel.service <<'EOF'
[Unit]
Description=Venom Tunnel v2.0 Web Panel & Orchestrator
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/venomd
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now venom-panel.service

printf "\n%s  ============================================================%s\n" "$G" "$N"
printf "  %sVENOM TUNNEL v2.0 INSTALLED SUCCESSFULLY%s\n" "$W" "$N"
printf "%s  ============================================================%s\n" "$G" "$N"
printf "  %sWeb Panel URL :%s %s%s%s\n" "$C" "$N" "$W" "$URL" "$N"
printf "  %sUsername      :%s %s%s%s\n" "$C" "$N" "$W" "$USER" "$N"
if [ -n "$PASS" ]; then
  printf "  %sPassword      :%s %s%s%s\n" "$C" "$N" "$G" "$PASS" "$N"
else
  printf "  %sPassword      :%s %s(kept existing — run 'venom reset-auth' to rotate)%s\n" "$C" "$N" "$Y" "$N"
fi
printf "  %sCLI Command   :%s %svenom%s\n" "$C" "$N" "$G" "$N"
printf "%s  ============================================================%s\n\n" "$G" "$N"
