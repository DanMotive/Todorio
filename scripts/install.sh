#!/usr/bin/env bash
# Todorio — ONE-COMMAND install (downloads a released binary + DB + systemd + setup):
#
#   curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/main/scripts/install.sh | sudo bash
#
# The binary is fully self-contained — the frontend and SQL migrations are
# embedded into it at release-build time (see /assets.go and
# .github/workflows/release.yml), so this script only needs to download one
# file and verify it. No Go or Node.js toolchain is installed on the server.
# `todorio update` later reuses the exact same download-and-verify logic
# (internal/ops/ops.go), so keep the asset naming and checksums.txt format here
# in sync with that if either ever changes.
#
# This installer is Debian-family only (it speaks apt). The binary itself is
# statically linked (CGO_ENABLED=0) and runs on any modern Linux — see the
# manual installation notes in README.md for RHEL, CentOS, Fedora and friends.
#
# When it finishes, the site is already running; the temporary root password is shown in the console.
set -euo pipefail

REPO="DanMotive/Todorio"
BIN="/usr/local/bin/todorio"

# Minimum PostgreSQL major version — a hard requirement, not a preference:
# migrations/0013_search_audit.sql adds `GENERATED ALWAYS AS (...) STORED` tsvector
# columns, which PostgreSQL only understands from 12 onwards. Nothing in migrations/
# needs anything newer. If this ever rises, raise the documented distribution
# minimums in README.md in the same commit: Ubuntu 20.04 ships PG 12, Debian 11
# ships PG 13, Ubuntu 22.04 ships PG 14, Debian 12 ships PG 15.
MIN_PG=12

say()  { printf '\033[1;36m[todorio]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[todorio] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# Installed PostgreSQL major version, or 0 when psql is not on PATH.
pg_major() {
  if command -v psql >/dev/null 2>&1; then
    psql -V | grep -oE '[0-9]+' | head -1
  else
    echo 0
  fi
}

[ "$(id -u)" -eq 0 ] || fail "Run with sudo or as root."

case "$(uname -s)" in
  Linux) : ;;
  *) fail "Unsupported OS: $(uname -s) — Todorio ships Linux binaries only." ;;
esac
case "$(uname -m)" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) fail "Unsupported architecture: $(uname -m)" ;;
esac
ASSET="todorio_linux_${ARCH}"

# Every package this script installs comes from apt. Say so plainly instead of dying
# on 'apt-get: command not found' a few lines below.
command -v apt-get >/dev/null 2>&1 || fail "This installer needs apt (Ubuntu 22.04+ / Debian 12+). Your distribution uses a different package manager — Todorio still runs there, but has to be installed manually: see 'Installation' in README.md."

export DEBIAN_FRONTEND=noninteractive
# needrestart on Ubuntu 22+ can hang apt behind an interactive dialog — silence it
export NEEDRESTART_MODE=a
export NEEDRESTART_SUSPEND=1

# --- 1. System dependencies (skip apt for anything already present): curl, jq, PostgreSQL ---
say "Checking dependencies..."
apt_missing=()
command -v curl >/dev/null 2>&1 || apt_missing+=(curl)
command -v jq >/dev/null 2>&1 || apt_missing+=(jq)
dpkg -s ca-certificates >/dev/null 2>&1 || apt_missing+=(ca-certificates)

PGV="$(pg_major)"
PG_OK=0
[ "${PGV:-0}" -ge "$MIN_PG" ] && PG_OK=1
[ "$PG_OK" -eq 1 ] || apt_missing+=(postgresql)

if [ "${#apt_missing[@]}" -gt 0 ]; then
  say "installing: ${apt_missing[*]} (may take a minute)..."
  apt-get update -y
  apt-get install -y "${apt_missing[@]}"
else
  say "curl, jq, ca-certificates, and PostgreSQL $PGV are already installed — skipping apt install."
fi

# Whatever apt just produced has to clear the bar as well. Without this re-check the
# script would go on to create a role and database on a version the migrations cannot
# run on, report success, and leave the failure to surface later from the server.
if [ "$PG_OK" -eq 0 ]; then
  PGV="$(pg_major)"
  if [ "${PGV:-0}" -eq 0 ]; then
    fail "PostgreSQL still is not available after installing it — check the 'apt-get install postgresql' output above."
  fi
  if [ "$PGV" -lt "$MIN_PG" ]; then
    fail "PostgreSQL $MIN_PG+ is required, but this system provides $PGV. Installing the 'postgresql' package does not upgrade an existing cluster, so an older server stays in place. Either add the PGDG repository (https://www.postgresql.org/download/linux/) and install postgresql-$MIN_PG or newer, or use Ubuntu 22.04+ / Debian 12+."
  fi
  say "PostgreSQL $PGV ready."
fi

# --- 2. PostgreSQL: user and database ---
say "Configuring PostgreSQL..."
systemctl enable --now postgresql >/dev/null 2>&1 || true
sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='todorio'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE USER todorio WITH PASSWORD 'todorio';"
sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='todorio'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE DATABASE todorio OWNER todorio;"

# --- 3. Download the latest release: binary + checksums.txt, verify sha256 ---
say "Fetching the latest release..."
RELEASE_JSON="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")" \
  || fail "Could not reach GitHub Releases (or no release has been published yet)."

TAG="$(printf '%s' "$RELEASE_JSON" | jq -r '.tag_name // "unknown"')"
BIN_URL="$(printf '%s' "$RELEASE_JSON" | jq -r --arg name "$ASSET" '.assets[]? | select(.name == $name) | .browser_download_url')"
SUM_URL="$(printf '%s' "$RELEASE_JSON" | jq -r '.assets[]? | select(.name == "checksums.txt") | .browser_download_url')"

[ -n "$BIN_URL" ] || fail "Release $TAG has no asset named $ASSET — has a release been published for this platform?"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
say "Downloading $ASSET ($TAG)..."
curl -fsSL -o "$TMP/todorio" "$BIN_URL"

if [ -n "$SUM_URL" ]; then
  curl -fsSL -o "$TMP/checksums.txt" "$SUM_URL"
  GOT="$(sha256sum "$TMP/todorio" | awk '{print $1}')"
  grep -q "$GOT" "$TMP/checksums.txt" || fail "sha256 mismatch for $ASSET — download is corrupted or tampered with. Aborting."
  say "sha256 verified"
else
  say "WARNING: release $TAG has no checksums.txt — installing without verification."
fi

install -m 0755 "$TMP/todorio" "$BIN"
mkdir -p /var/lib/todorio/uploads /var/lib/todorio/backups /etc/todorio

# --- 4. systemd unit (no WorkingDirectory needed — the binary is self-contained and every path it uses is absolute) ---
say "Installing systemd service..."
cat > /etc/systemd/system/todorio.service <<EOF
[Unit]
Description=Todorio — todo server
After=network.target postgresql.service
Wants=postgresql.service

[Service]
ExecStart=$BIN serve
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload

# --- 5. First-time setup — skipped if this machine is already configured, so re-running this
# script (e.g. to fetch an updated binary) never re-prompts an already-set-up instance. Setup
# only runs automatically on a genuinely fresh install; afterwards it's opt-in via `sudo todorio
# setup` (works even via curl | bash, thanks to /dev/tty for the fresh-install case).
if [ -f /etc/todorio/config.json ]; then
  say "Existing installation detected (/etc/todorio/config.json) — skipping setup."
  systemctl restart todorio
  say "Done! todorio restarted with this binary. Check: todorio status"
elif [ -e /dev/tty ] && [ -r /dev/tty ]; then
  say "Running first-time setup..."
  "$BIN" setup < /dev/tty
  systemctl enable --now todorio
  say "Done! The site is running. Check: todorio status"
else
  say "Done! Next: sudo todorio setup && sudo systemctl enable --now todorio"
fi
say "To remove Todorio later: sudo todorio uninstall (add --purge to also delete application data and the database, --saveconfig to keep /etc/todorio)"
say "Day-to-day service control: sudo todorio start / stop / restart"
