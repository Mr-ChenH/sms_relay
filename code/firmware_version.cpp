#include "firmware_version.h"

__attribute__((used)) const SMSHubFirmwareMetadata SMSHUB_FIRMWARE_METADATA = {
  {'S', 'M', 'S', 'H', 'U', 'B', 'F', 'W'},
  SMSHUB_FIRMWARE_VERSION,
  SMSHUB_HARDWARE_MODEL,
};
