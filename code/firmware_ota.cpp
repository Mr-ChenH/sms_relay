#include "firmware_ota.h"
#include "firmware_version.h"

#include <HTTPClient.h>
#include <Update.h>
#include <WiFiClient.h>
#include <esp_ota_ops.h>
#include <mbedtls/sha256.h>

static const char* OTA_HARDWARE_MODEL = SMSHUB_HARDWARE_MODEL;
static const size_t OTA_BUFFER_SIZE = 4096;
static const unsigned long OTA_READ_TIMEOUT_MS = 30000;

static String hexDigest(const unsigned char* digest, size_t length) {
  static const char digits[] = "0123456789abcdef";
  String value;
  value.reserve(length * 2);
  for (size_t i = 0; i < length; ++i) {
    value += digits[digest[i] >> 4];
    value += digits[digest[i] & 0x0f];
  }
  return value;
}

bool firmwareOTAExecute(const String& url, const String& version, size_t expectedSize,
                        const String& expectedSHA256, const String& expectedHardware,
                        String& result) {
  if (!url.startsWith("http://")) {
    result = "OTA URL must use http:// on this firmware";
    return false;
  }
  if (expectedHardware.length() > 0 && expectedHardware != OTA_HARDWARE_MODEL) {
    result = "firmware hardware model does not match this terminal";
    return false;
  }
  if (expectedSize == 0 || expectedSHA256.length() != 64) {
    result = "invalid OTA size or SHA-256";
    return false;
  }

  const esp_partition_t* target = esp_ota_get_next_update_partition(nullptr);
  if (!target) {
    result = "no OTA partition is available; flash the OTA partition table over USB once";
    return false;
  }
  if (expectedSize > target->size) {
    result = "firmware image is larger than the OTA partition";
    return false;
  }

  WiFiClient client;
  HTTPClient http;
  http.setConnectTimeout(10000);
  http.setTimeout(OTA_READ_TIMEOUT_MS);
  if (!http.begin(client, url)) {
    result = "unable to initialize OTA download";
    return false;
  }
  int status = http.GET();
  if (status != HTTP_CODE_OK) {
    result = String("OTA download failed with HTTP ") + status;
    http.end();
    return false;
  }
  int contentLength = http.getSize();
  if (contentLength < 0 || static_cast<size_t>(contentLength) != expectedSize) {
    result = "OTA Content-Length does not match the signed metadata";
    http.end();
    return false;
  }
  if (!Update.begin(expectedSize, U_FLASH)) {
    result = String("OTA partition initialization failed: ") + Update.errorString();
    http.end();
    return false;
  }

  mbedtls_sha256_context sha;
  mbedtls_sha256_init(&sha);
  mbedtls_sha256_starts(&sha, 0);
  uint8_t* buffer = static_cast<uint8_t*>(malloc(OTA_BUFFER_SIZE));
  if (!buffer) {
    mbedtls_sha256_free(&sha);
    Update.abort();
    http.end();
    result = "insufficient heap for OTA buffer";
    return false;
  }

  WiFiClient* stream = http.getStreamPtr();
  size_t received = 0;
  unsigned long lastDataAt = millis();
  bool failed = false;
  while (received < expectedSize) {
    int available = stream->available();
    if (available <= 0) {
      if (!http.connected() || millis() - lastDataAt > OTA_READ_TIMEOUT_MS) {
        result = "OTA download interrupted";
        failed = true;
        break;
      }
      delay(5);
      continue;
    }
    size_t chunk = min(static_cast<size_t>(available), min(OTA_BUFFER_SIZE, expectedSize - received));
    int count = stream->readBytes(buffer, chunk);
    if (count <= 0) continue;
    lastDataAt = millis();
    mbedtls_sha256_update(&sha, buffer, count);
    if (Update.write(buffer, count) != static_cast<size_t>(count)) {
      result = String("OTA flash write failed: ") + Update.errorString();
      failed = true;
      break;
    }
    received += count;
    delay(1);
  }

  unsigned char digest[32];
  mbedtls_sha256_finish(&sha, digest);
  mbedtls_sha256_free(&sha);
  free(buffer);
  http.end();

  if (failed || received != expectedSize) {
    Update.abort();
    return false;
  }
  String actualSHA256 = hexDigest(digest, sizeof(digest));
  String normalizedExpected = expectedSHA256;
  normalizedExpected.toLowerCase();
  if (actualSHA256 != normalizedExpected) {
    Update.abort();
    result = "OTA SHA-256 verification failed";
    return false;
  }
  if (!Update.end(true)) {
    result = String("OTA finalization failed: ") + Update.errorString();
    return false;
  }
  result = String("firmware ") + version + " installed and verified; rebooting";
  return true;
}

void firmwareOTAMarkValid() {
  const esp_partition_t* running = esp_ota_get_running_partition();
  esp_ota_img_states_t state;
  if (running && esp_ota_get_state_partition(running, &state) == ESP_OK && state == ESP_OTA_IMG_PENDING_VERIFY) {
    esp_ota_mark_app_valid_cancel_rollback();
  }
}
