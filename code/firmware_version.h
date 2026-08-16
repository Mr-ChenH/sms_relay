#pragma once

#define SMSHUB_FIRMWARE_VERSION "0.10.0-terminal"
#define SMSHUB_HARDWARE_MODEL "ESP32-C3 + ML307A"

struct SMSHubFirmwareMetadata {
  char magic[8];
  char version[32];
  char hardware[32];
};

extern const SMSHubFirmwareMetadata SMSHUB_FIRMWARE_METADATA;
