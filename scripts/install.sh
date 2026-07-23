#!/usr/bin/env bash
# Todorio — ONE-COMMAND install (build from source + DB + systemd + setup):
#
#   curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/main/scripts/install.sh | sudo bash
#
# When it finishes, the site is already running; the temporary root password is shown in the console.
say "Starting installation..."
set -euo pipefail

REPO="https://github.com/DanMotive/Todorio.git"
BRANCH="${TODORIO_BRANCH:-main}"
SRC_DIR="/opt/todorio-src"
BIN="/usr/local/bin/todorio"
SHARE="/usr/share/todorio"
GO_TARBALL_VER="1.22.5"

say()  { printf '\033[1;36m[todorio]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[todorio] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "Run with sudo or as root."

case "$(uname -m)" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) fail "Unsupported architecture: $(uname -m)" ;;
esac

export DEBIAN_FRONTEND=noninteractive
# needrestart on Ubuntu 22+ can hang apt behind an interactive dialog — silence it
export NEEDRESTART_MODE=a
export NEEDRESTART_SUSPEND=1

# --- 1. System dependencies (skip apt/download for anything already at a suitable version) ---
say "Checking dependencies..."

# git / curl / ca-certificates: only queue for apt if actually missing
apt_missing=()
command -v git >/dev/null 2>&1 || apt_missing+=(git)
command -v curl >/dev/null 2>&1 || apt_missing+=(curl)
dpkg -s ca-certificates >/dev/null 2>&1 || apt_missing+=(ca-certificates)

# PostgreSQL: skip apt install if a server/client >=14 is already present
PG_OK=0
PGV=0
if command -v psql >/dev/null 2>&1; then
  PGV="$(psql -V | grep -oE '[0-9]+' | head -1)"
  [ "${PGV:-0}" -ge 14 ] && PG_OK=1
fi
[ "$PG_OK" -eq 1 ] || apt_missing+=(postgresql)

if [ "${#apt_missing[@]}" -gt 0 ]; then
  say "installing: ${apt_missing[*]} (may take a couple of minutes, progress below)..."
  apt-get update -y
  apt-get install -y "${apt_missing[@]}"
else
  say "git, curl, ca-certificates, and PostgreSQL $PGV are already installed at suitable versions — skipping apt install."
fi

# Node.js 18+ (to build the Vite frontend) — skip install if already adequate
NODE_MAJOR="$(node -v 2>/dev/null | sed 's/^v\([0-9]*\).*/\1/' || echo 0)"
if [ "${NODE_MAJOR:-0}" -ge 18 ]; then
  say "Node.js $(node -v) is already adequate — skipping install/download."
else
  say "Installing Node.js 20 (the apt version is too old)..."
  curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
  apt-get install -y nodejs
fi

# Go 1.22+ — reuse an existing toolchain (PATH or /usr/local/go) if it's new enough, otherwise download once
GO="go"
GO_OK=0
GO_BIN=""
if command -v go >/dev/null 2>&1; then
  GO_BIN="$(command -v go)"
elif [ -x /usr/local/go/bin/go ]; then
  GO_BIN="/usr/local/go/bin/go"
fi
if [ -n "$GO_BIN" ]; then
  GV="$("$GO_BIN" version | grep -oE 'go[0-9]+\.[0-9]+' | head -1 | tr -d 'go')"
  MAJOR="${GV%%.*}"; MINOR="${GV#*.}"
  if [ "${MAJOR:-0}" -gt 1 ] || { [ "${MAJOR:-0}" -eq 1 ] && [ "${MINOR:-0}" -ge 22 ]; }; then GO_OK=1; fi
fi
if [ "$GO_OK" -eq 1 ]; then
  GO="$GO_BIN"
  say "$("$GO" version | awk '{print $3}') is already adequate — skipping download."
else
  say "Installing Go $GO_TARBALL_VER..."
  curl -fsSL -o /tmp/go.tgz "{{https://go.dev/dl/go${GO_TARBALL_VER}}}.linux-${ARCH}.tar.gz"
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
  GO="/usr/local/go/bin/go"
fi

# --- 2. PostgreSQL: user and database ---
say "Configuring PostgreSQL..."
systemctl enable --now postgresql >/dev/null 2>&1 || true
sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='todorio'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE USER todorio WITH PASSWORD 'todorio';"
sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='todorio'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE DATABASE todorio OWNER todorio;"

# --- 3. Source ---
if [ -d "$SRC_DIR/.git" ]; then
  say "Updating source ($BRANCH)..."
  git -C "$SRC_DIR" fetch --depth 1 origin "$BRANCH"
  git -C "$SRC_DIR" reset --hard "origin/$BRANCH"
else
  say "Cloning $REPO ($BRANCH)..."
  rm -rf "$SRC_DIR"
  git clone --depth 1 -b "$BRANCH" "$REPO" "$SRC_DIR"
fi
cd "$SRC_DIR"

# --- 4. Build ---
say "Building backend (Go)..."
"$GO" mod tidy
CGO_ENABLED=0 "$GO" build -o /tmp/todorio-bin ./cmd/todorio

say "Building frontend (React + Vite)..."
cd web
npm install --no-audit --no-fund --loglevel=error
npm run build --silent
cd ..

# --- 5. Installing files ---
say "Installing into the system..."
install -m 0755 /tmp/todorio-bin "$BIN" && rm -f /tmp/todorio-bin
mkdir -p "$SHARE/web" /var/lib/todorio/uploads /var/lib/todorio/backups /etc/todorio
rm -rf "$SHARE/migrations" "$SHARE/web/dist"
cp -r migrations "$SHARE/migrations"
cp -r web/dist "$SHARE/web/dist"

# --- 6. systemd unit ---
cat > /etc/systemd/system/todorio.service <<EOF
[Unit]
Description=Todorio — todo server
After=network.target postgresql.service
Wants=postgresql.service

[Service]
ExecStart=$BIN serve
WorkingDirectory=$SHARE
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload

# --- 7. Interactive setup (works even via curl | bash, thanks to /dev/tty) ---
if [ -e /dev/tty ] && [ -r /dev/tty ]; then
  say "Running first-time setup..."
  "$BIN" setup < /dev/tty
  systemctl enable --now todorio
  say "Done! The site is running. Check: todorio doctor"
else
  say "Done! Next: sudo todorio setup && sudo systemctl enable --now todorio"
fi
say "To remove Todorio later: sudo todorio uninstall (add --purge to also delete application data and the database, --saveconfig to keep /etc/todorio)"
