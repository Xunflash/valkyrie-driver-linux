#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Building valkyrie-driver (pure Go, CGO_ENABLED=0)..."
make install

if [[ ! -f /etc/udev/rules.d/99-valkyrie-hid.rules ]]; then
  echo "==> Installing udev rules (requires sudo)..."
  make install-udev
else
  echo "==> /etc/udev/rules.d/99-valkyrie-hid.rules already exists, skipping."
fi

echo "==> Done! Launch 'Valkyrie Driver' from your app menu or run:"
echo "    ~/.local/bin/valkyrie-driver"
echo "    ~/.local/bin/valkyrie-driver --status"
