#!/usr/bin/env bash
# Todorio installer — https://github.com/DanMotive/Todorio
# Usage: curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/v1.0.0/scripts/install.sh | sudo bash
set -euo pipefail

REPO="DanMotive/Todorio"
VERSION="${TODORIO_VERSION:-v1.0.0}"
BIN_DIR="/usr/local/bin"
DATA_DIR="/var/lib/todorio"

say()  { printf '\033[1;36m[todorio]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[todorio] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "Запустите через sudo или от root."

# --- ОС и архитектура ---
. /etc/os-release 2>/dev/null || fail "Не удалось определить ОС."
case "$ID" in
  ubuntu|debian) : ;;
  *) say "Внимание: тестировалось на Ubuntu/Debian (у вас: $ID). Продолжаем..." ;;
esac

case "$(uname -m)" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) fail "Неподдерживаемая архитектура: $(uname -m)" ;;
esac

# --- Скачивание релиза и проверка контрольной суммы ---
BASE="https://github.com/$REPO/releases/download/$VERSION"
TARBALL="todorio_${VERSION#v}_linux_${ARCH}.tar.gz"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

say "Скачиваю $TARBALL ($VERSION)..."
curl -fsSL -o "$TMP/$TARBALL" "$BASE/$TARBALL"
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt"

say "Проверяю SHA-256..."
(cd "$TMP" && grep " $TARBALL\$" checksums.txt | sha256sum -c -) || fail "Контрольная сумма не совпала — установка прервана."

say "Устанавливаю бинарник в $BIN_DIR..."
tar -xzf "$TMP/$TARBALL" -C "$TMP"
install -m 0755 "$TMP/todorio" "$BIN_DIR/todorio"

mkdir -p "$DATA_DIR/uploads" /etc/todorio

say "Готово! Todorio $VERSION установлен."
say "Дальше: sudo todorio setup"
