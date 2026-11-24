#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $0 <binary_path> <runtime_file> <output_appimage>" >&2
  exit 1
fi

BIN_PATH="$1"
RUNTIME_FILE="$2"
OUT_APPIMAGE="$3"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ ! -f "$BIN_PATH" ]]; then
  echo "Binary not found: $BIN_PATH" >&2
  exit 1
fi
if [[ ! -f "$RUNTIME_FILE" ]]; then
  echo "AppImage Type 2 runtime not found: $RUNTIME_FILE" >&2
  exit 1
fi
if ! command -v mksquashfs >/dev/null 2>&1; then
  echo "mksquashfs not found (install squashfs-tools)" >&2
  exit 1
fi

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

APPDIR="${WORK_DIR}/ValkyrieDriver.AppDir"
mkdir -p \
  "${APPDIR}/usr/bin" \
  "${APPDIR}/usr/share/applications" \
  "${APPDIR}/usr/share/icons/hicolor/256x256/apps" \
  "${APPDIR}/usr/share/valkyrie-driver" \
  "${APPDIR}/usr/share/doc/valkyrie-driver"

install -m 0755 "$BIN_PATH" "${APPDIR}/usr/bin/valkyrie-driver"
install -m 0644 "${ROOT_DIR}/assets/icon.png" "${APPDIR}/valkyrie-driver.png"
install -m 0644 "${ROOT_DIR}/assets/icon.png" "${APPDIR}/usr/share/icons/hicolor/256x256/apps/valkyrie-driver.png"
ln -sf valkyrie-driver.png "${APPDIR}/.DirIcon"
install -m 0644 "${ROOT_DIR}/packaging/99-valkyrie-hid.rules" "${APPDIR}/usr/share/valkyrie-driver/99-valkyrie-hid.rules"
install -m 0644 "${ROOT_DIR}/LICENSE" "${APPDIR}/usr/share/doc/valkyrie-driver/LICENSE"

sed "s|@EXEC@|valkyrie-driver|g; s|@ICON@|valkyrie-driver|g" \
  "${ROOT_DIR}/packaging/valkyrie-driver.desktop" \
  > "${APPDIR}/valkyrie-driver.desktop"
chmod 0644 "${APPDIR}/valkyrie-driver.desktop"
cp "${APPDIR}/valkyrie-driver.desktop" "${APPDIR}/usr/share/applications/valkyrie-driver.desktop"

if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "${APPDIR}/valkyrie-driver.desktop"
fi

cat <<'EOF' > "${APPDIR}/AppRun"
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "${HERE}/usr/bin/valkyrie-driver" "$@"
EOF
chmod 0755 "${APPDIR}/AppRun"

SQUASHFS="${WORK_DIR}/app.squashfs"
mksquashfs "$APPDIR" "$SQUASHFS" -root-owned -noappend -comp gzip -quiet

mkdir -p "$(dirname "$OUT_APPIMAGE")"
cat "$RUNTIME_FILE" "$SQUASHFS" > "$OUT_APPIMAGE"
chmod 0755 "$OUT_APPIMAGE"
echo "Built AppImage: $OUT_APPIMAGE"
