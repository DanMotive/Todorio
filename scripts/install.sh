#!/usr/bin/env bash
# Todorio — установка ОДНОЙ командой (сборка из исходников + БД + systemd + setup):
#
#   curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/main/scripts/install.sh | sudo bash
#
# После завершения сайт уже запущен, временный пароль рута показан в консоли.
set -euo pipefail

REPO="https://github.com/DanMotive/Todorio.git"
BRANCH="${TODORIO_BRANCH:-main}"
SRC_DIR="/opt/todorio-src"
BIN="/usr/local/bin/todorio"
SHARE="/usr/share/todorio"
GO_TARBALL_VER="1.22.5"

say()  { printf '\033[1;36m[todorio]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[todorio] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "Запустите через sudo или от root."

case "$(uname -m)" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) fail "Неподдерживаемая архитектура: $(uname -m)" ;;
esac

export DEBIAN_FRONTEND=noninteractive

# --- 1. Системные зависимости ---
say "Ставлю зависимости (git, PostgreSQL, Node.js)..."
apt-get update -y -qq
apt-get install -y -qq git curl ca-certificates build-essential postgresql >/dev/null

# Node.js 18+ (для сборки фронтенда Vite)
NODE_MAJOR="$(node -v 2>/dev/null | sed 's/^v\([0-9]*\).*/\1/' || echo 0)"
if [ "${NODE_MAJOR:-0}" -lt 18 ]; then
  say "Ставлю Node.js 20 (в apt слишком старый)..."
  curl -fsSL https://deb.nodesource.com/setup_20.x | bash - >/dev/null
  apt-get install -y -qq nodejs >/dev/null
fi

    curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GO_TARBALL_VER}.linux-${ARCH}.tar.gz"
GO="go"
GO_OK=0
if command -v go >/dev/null 2>&1; then
  GV="$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1 | tr -d 'go')"
  MAJOR="${GV%%.*}"; MINOR="${GV#*.}"
  if [ "${MAJOR:-0}" -gt 1 ] || { [ "${MAJOR:-0}" -eq 1 ] && [ "${MINOR:-0}" -ge 22 ]; }; then GO_OK=1; fi
fi
if [ "$GO_OK" -ne 1 ]; then
  if [ -x /usr/local/go/bin/go ]; then
    GO="/usr/local/go/bin/go"
  else
    say "Ставлю Go $GO_TARBALL_VER..."
    curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GO_TARBALL_VER}.linux-${ARCH}.tar.gz"
    rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
    GO="/usr/local/go/bin/go"
  fi
fi

# --- 2. PostgreSQL: пользователь и база ---
say "Настраиваю PostgreSQL..."
systemctl enable --now postgresql >/dev/null 2>&1 || true
sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='todorio'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE USER todorio WITH PASSWORD 'todorio';"
sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='todorio'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE DATABASE todorio OWNER todorio;"

# --- 3. Исходники ---
if [ -d "$SRC_DIR/.git" ]; then
  say "Обновляю исходники ($BRANCH)..."
  git -C "$SRC_DIR" fetch --depth 1 origin "$BRANCH"
  git -C "$SRC_DIR" reset --hard "origin/$BRANCH"
else
  say "Клонирую $REPO ($BRANCH)..."
  rm -rf "$SRC_DIR"
  git clone --depth 1 -b "$BRANCH" "$REPO" "$SRC_DIR"
fi
cd "$SRC_DIR"

# --- 4. Сборка ---
say "Собираю бэкенд (Go)..."
"$GO" mod tidy >/dev/null
"$GO" build -o /tmp/todorio-bin ./cmd/todorio

say "Собираю фронтенд (React + Vite)..."
cd web
npm install --no-audit --no-fund --loglevel=error
npm run build --silent
cd ..

# --- 5. Установка файлов ---
say "Устанавливаю в систему..."
install -m 0755 /tmp/todorio-bin "$BIN" && rm -f /tmp/todorio-bin
mkdir -p "$SHARE/web" /var/lib/todorio/uploads /var/lib/todorio/backups /etc/todorio
rm -rf "$SHARE/migrations" "$SHARE/web/dist"
cp -r migrations "$SHARE/migrations"
cp -r web/dist "$SHARE/web/dist"

# --- 6. systemd-юнит ---
cat > /etc/systemd/system/todorio.service <<EOF
[Unit]
Description=Todorio — todo-сервер
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

# --- 7. Интерактивный setup (работает даже через curl | bash благодаря /dev/tty) ---
if [ -e /dev/tty ] && [ -r /dev/tty ]; then
  say "Запускаю первичную настройку..."
  "$BIN" setup < /dev/tty
  systemctl enable --now todorio
  say "Готово! Сайт запущен. Проверка: todorio doctor"
else
  say "Готово! Дальше: sudo todorio setup && sudo systemctl enable --now todorio"
fi
