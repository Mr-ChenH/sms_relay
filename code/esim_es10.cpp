#include "esim_es10.h"
#include "esim_at.h"
#include "esim_internal.h"
#include "esim_tlv.h"
#include "logger.h"

static const uint8_t ISD_R_AID[] = {
  0xA0, 0x00, 0x00, 0x05, 0x59, 0x10, 0x10, 0xFF,
  0xFF, 0xFF, 0xFF, 0x89, 0x00, 0x00, 0x01, 0x00
};

static bool openChannel(String* channel) {
  String aid = esimBytesToHex(ISD_R_AID, sizeof(ISD_R_AID));
  String response;
  for (int attempt = 0; attempt < 2; attempt++) {
    response = esimSendAT((String("AT+CCHO=\"") + aid + "\"").c_str(), 10000);
    int id = 0;
    if (esimParseChannelResponse(response, &id)) {
      *channel = String(id);
      return true;
    }
    if (attempt == 0) {
      // Some cards answer after modem-ready URCs or leave a stale logical
      // channel behind. Close known channels and retry once.
      for (int id = 1; id <= 8; id++) esimSendAT((String("AT+CCHC=") + id).c_str(), 2000);
      delay(100);
    }
  }
  esimSetError(String("打开 eUICC 通道失败: ") + esimCompactATResponse(response));
  return false;
}

static void closeChannel(const String& channel) {
  if (channel.length() > 0) esimSendAT((String("AT+CCHC=") + channel).c_str(), 5000);
}

static bool transmit(const String& channel, const uint8_t* tx, size_t txLength, uint8_t** rx, size_t* rxLength) {
  *rx = nullptr;
  *rxLength = 0;
  String txHex = esimBytesToHex(tx, txLength);
  String command = "AT+CGLA=" + channel + "," + String(txHex.length()) + ",\"" + txHex + "\"";
  String response = esimSendAT(command.c_str(), 30000);
  String hex;
  if (!esimParseCGLAHexPayload(response, &hex)) {
    esimSetError(String("APDU 传输失败: ") + esimCompactATResponse(response));
    return false;
  }
  if (!esimIsHexString(hex)) {
    String compacted = esimPrintableHex(hex);
    String longest;
    if (esimIsHexString(compacted) && compacted.length() >= 4) hex = compacted;
    else if (esimExtractLongestHexRun(hex, &longest)) hex = longest;
  }
  size_t capacity = hex.length() / 2;
  uint8_t* buffer = (uint8_t*)malloc(capacity);
  if (!buffer) {
    esimSetError("内存不足，无法接收 APDU 响应");
    return false;
  }
  if (!esimHexToBytes(hex, buffer, capacity, rxLength)) {
    free(buffer);
    esimSetError(String("CGLA 响应不是合法 HEX: ") + esimPrintableHex(hex));
    return false;
  }
  *rx = buffer;
  return true;
}

static bool appendResponse(uint8_t** output, size_t* outputLength, const uint8_t* data, size_t length) {
  if (length == 0) return true;
  uint8_t* next = (uint8_t*)realloc(*output, *outputLength + length);
  if (!next) {
    free(*output);
    *output = nullptr;
    *outputLength = 0;
    esimSetError("内存不足，无法拼接 eUICC 响应");
    return false;
  }
  memcpy(next + *outputLength, data, length);
  *output = next;
  *outputLength += length;
  return true;
}

static uint8_t classByte(const String& channel) {
  int id = channel.toInt();
  if (id <= 0 || id > 19) return 0x80;
  if (id < 4) return (0x80 & 0x9C) | id;
  return (0x80 & 0xB0) | 0x40 | (id - 4);
}

bool esimES10Command(const uint8_t* request, size_t requestLength, uint8_t** response, size_t* responseLength) {
  if (!request || !response || !responseLength) return false;
  *response = nullptr;
  *responseLength = 0;
  if (requestLength > 255) {
    esimSetError("请求过长，当前实现只支持短 APDU");
    return false;
  }

  String channel;
  if (!openChannel(&channel)) return false;
  bool ok = false;
  uint8_t apdu[260];
  uint8_t cla = classByte(channel);
  apdu[0] = cla;
  apdu[1] = 0xE2;
  apdu[2] = 0x91;
  apdu[3] = 0x00;
  apdu[4] = (uint8_t)requestLength;
  memcpy(apdu + 5, request, requestLength);

  int getResponseRounds = 0;
  while (true) {
    uint8_t* rx = nullptr;
    size_t rxLength = 0;
    if (!transmit(channel, apdu, 5 + requestLength, &rx, &rxLength)) goto done;
    if (rxLength < 2) {
      free(rx);
      esimSetError("APDU 响应长度不足");
      goto done;
    }
    uint8_t sw1 = rx[rxLength - 2];
    uint8_t sw2 = rx[rxLength - 1];
    if (!appendResponse(response, responseLength, rx, rxLength - 2)) {
      free(rx);
      goto done;
    }
    free(rx);
    if (sw1 == 0x61) {
      // eUICC 异常时可能反复返回 0x61，必须有硬上限防止无限循环
      if (++getResponseRounds > 32) {
        esimSetError("APDU GET RESPONSE 轮数超限，终止操作");
        goto done;
      }
      apdu[0] = cla;
      apdu[1] = 0xC0;
      apdu[2] = 0;
      apdu[3] = 0;
      apdu[4] = sw2;
      requestLength = 0;
      continue;
    }
    if ((sw1 & 0xF0) == 0x90) {
      ok = true;
      break;
    }
    char error[64];
    snprintf(error, sizeof(error), "APDU 状态字错误: %02X%02X", sw1, sw2);
    esimSetError(error);
    goto done;
  }

done:
  closeChannel(channel);
  if (!ok) {
    free(*response);
    *response = nullptr;
    *responseLength = 0;
  }
  return ok;
}
