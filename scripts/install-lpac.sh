#!/usr/bin/env bash
set -euo pipefail

LPAC_VERSION="v2.2.1"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="${LPAC_INSTALL_DIR:-$ROOT/server/tools/lpac}"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

case "$(uname -s):$(uname -m)" in
  Linux:x86_64|Linux:amd64)
    archive="lpac-linux-x86_64-without-lto.zip"
    checksum="5afdf0c3490e2d04cf4432375a296e68a6374c486c7bac1779ca75d4bb96b33b"
    ;;
  *)
    echo "Unsupported platform: $(uname -s) $(uname -m). Use Docker or install lpac manually." >&2
    exit 1
    ;;
esac

for command in curl sha256sum unzip; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "Required command not found: $command" >&2
    exit 1
  fi
done

url="https://github.com/estkme-group/lpac/releases/download/${LPAC_VERSION}/${archive}"
curl --fail --location --show-error --silent --output "$TEMP_DIR/lpac.zip" "$url"
printf '%s  %s\n' "$checksum" "$TEMP_DIR/lpac.zip" | sha256sum --check --status

mkdir -p "$INSTALL_DIR"
unzip -oq "$TEMP_DIR/lpac.zip" -d "$INSTALL_DIR"
chmod 0755 "$INSTALL_DIR/lpac"

printf 'Installed lpac %s at %s\n' "$LPAC_VERSION" "$INSTALL_DIR/lpac"
printf 'License files are available in %s\n' "$INSTALL_DIR"
