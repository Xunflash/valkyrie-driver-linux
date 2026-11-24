#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "Usage: $0 <binary_path> <arch: amd64|arm64> <version> <output_deb>" >&2
  exit 1
fi

BIN_PATH="$1"
ARCH="$2"
RAW_VERSION="$3"
OUT_DEB="$4"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${RAW_VERSION#v}"

case "$ARCH" in
  amd64|arm64) ;;
  *)
    echo "Unsupported Debian arch: $ARCH (expected amd64 or arm64)" >&2
    exit 1
    ;;
esac

if [[ ! -f "$BIN_PATH" ]]; then
  echo "Binary not found: $BIN_PATH" >&2
  exit 1
fi

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

PKG_ROOT="${WORK_DIR}/pkg"
mkdir -p \
  "${PKG_ROOT}/DEBIAN" \
  "${PKG_ROOT}/usr/bin" \
  "${PKG_ROOT}/usr/share/applications" \
  "${PKG_ROOT}/usr/share/icons/hicolor/256x256/apps" \
  "${PKG_ROOT}/usr/share/doc/valkyrie-driver" \
  "${PKG_ROOT}/lib/udev/rules.d"

install -m 0755 "$BIN_PATH" "${PKG_ROOT}/usr/bin/valkyrie-driver"
install -m 0644 "${ROOT_DIR}/assets/icon.png" "${PKG_ROOT}/usr/share/icons/hicolor/256x256/apps/valkyrie-driver.png"
install -m 0644 "${ROOT_DIR}/packaging/99-valkyrie-hid.rules" "${PKG_ROOT}/lib/udev/rules.d/99-valkyrie-hid.rules"
install -m 0644 "${ROOT_DIR}/LICENSE" "${PKG_ROOT}/usr/share/doc/valkyrie-driver/copyright"

sed "s|@EXEC@|/usr/bin/valkyrie-driver|g; s|@ICON@|valkyrie-driver|g" \
  "${ROOT_DIR}/packaging/valkyrie-driver.desktop" \
  > "${PKG_ROOT}/usr/share/applications/valkyrie-driver.desktop"
chmod 0644 "${PKG_ROOT}/usr/share/applications/valkyrie-driver.desktop"

INSTALLED_KB="$(du -sk "${PKG_ROOT}/usr" "${PKG_ROOT}/lib" | awk '{s+=$1} END {print s}')"

cat <<EOF > "${PKG_ROOT}/DEBIAN/control"
Package: valkyrie-driver
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${ARCH}
Maintainer: Xunflash <71567536+Xunflash@users.noreply.github.com>
Homepage: https://github.com/Xunflash/valkyrie-driver-linux
Installed-Size: ${INSTALLED_KB}
Depends: libgtk-3-0t64 | libgtk-3-0, libepoxy0, libpangocairo-1.0-0, libcairo2, libgl1 | libegl1
Recommends: udev
Description: Lightweight native Linux driver for Valkyrie gaming mice
 Pure-Go + MyGo native GPU configuration studio for Valkyrie (RongYuan
 CommonMSCH585) mice supporting Bluetooth (BLE), 2.4G wireless dongle, and
 USB wired connections. Includes udev rules for non-root /dev/hidraw access.
EOF
chmod 0644 "${PKG_ROOT}/DEBIAN/control"

cat <<'EOF' > "${PKG_ROOT}/DEBIAN/postinst"
#!/bin/sh
set -e
if command -v udevadm >/dev/null 2>&1; then
    udevadm control --reload-rules >/dev/null 2>&1 || true
    udevadm trigger --subsystem-match=hidraw --subsystem-match=usb >/dev/null 2>&1 || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi
EOF
chmod 0755 "${PKG_ROOT}/DEBIAN/postinst"

cat <<'EOF' > "${PKG_ROOT}/DEBIAN/postrm"
#!/bin/sh
set -e
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    if command -v udevadm >/dev/null 2>&1; then
        udevadm control --reload-rules >/dev/null 2>&1 || true
    fi
    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database -q /usr/share/applications || true
    fi
fi
EOF
chmod 0755 "${PKG_ROOT}/DEBIAN/postrm"

find "${PKG_ROOT}" -type d -exec chmod 0755 {} +
mkdir -p "$(dirname "$OUT_DEB")"
dpkg-deb --root-owner-group --build "$PKG_ROOT" "$OUT_DEB"
echo "Built DEB: $OUT_DEB"
