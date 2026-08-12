#!/usr/bin/env bash
set -euo pipefail

ARDUINO_CLI="${ARDUINO_CLI:-arduino-cli}"
FQBN="${FQBN:-esp32:esp32:esp32c3:PartitionScheme=min_spiffs}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP_DIR="$(mktemp -d)"
SKETCH_DIR="$TEMP_DIR/sms_forwarding"
trap 'rm -rf "$TEMP_DIR"' EXIT

mkdir -p "$SKETCH_DIR"
cp "$ROOT"/code/* "$SKETCH_DIR/"
mv "$SKETCH_DIR/code.ino" "$SKETCH_DIR/sms_forwarding.ino"
"$ARDUINO_CLI" compile --fqbn "$FQBN" "$SKETCH_DIR"
