#!/usr/bin/env bash
set -euo pipefail

ARDUINO_CLI="${ARDUINO_CLI:-arduino-cli}"
# 默认分区即可：固件已移除 BLE（改用 SoftAP 网页配网），当前约 1.17MB
# （默认 1.2MB APP）。若未来体积膨胀导致编译失败，优先考虑代码瘦身。
FQBN="${FQBN:-esp32:esp32:esp32c3}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP_DIR="$(mktemp -d)"
SKETCH_DIR="$TEMP_DIR/sms_forwarding"
trap 'rm -rf "$TEMP_DIR"' EXIT

mkdir -p "$SKETCH_DIR"
cp "$ROOT"/code/* "$SKETCH_DIR/"
mv "$SKETCH_DIR/code.ino" "$SKETCH_DIR/sms_forwarding.ino"
"$ARDUINO_CLI" compile --fqbn "$FQBN" "$SKETCH_DIR"
