#include "esim_at.h"
#include "esim_tlv.h"
#include "logger.h"
#include "sms_process.h"
#include "terminal_client.h"

#include <ctype.h>

String esimSendAT(const char* command, unsigned long timeout) {
  drainSerial1Urx();  // 先处理待收 URC，避免清空串口导致短信丢失
  Serial1.println(command);

  unsigned long start = millis();
  unsigned long lastByteAt = start;
  bool sawFinal = false;
  String response;
  response.reserve(2048);
  while (millis() - start < timeout) {
    terminalClientService();
    bool gotByte = false;
    while (Serial1.available()) {
      response += (char)Serial1.read();
      gotByte = true;
      lastByteAt = millis();
    }
    if (gotByte) {
      // 只检查末尾小窗口，避免每次复制整个响应串（O(n²)）
      int len = response.length();
      int from = len > 64 ? len - 64 : 0;
      String tail = response.substring(from);
      tail.trim();
      sawFinal = tail.endsWith("OK") || tail.endsWith("ERROR") ||
                 tail.indexOf("+CME ERROR:") >= 0 || tail.indexOf("+CMS ERROR:") >= 0;
    }
    if (sawFinal && millis() - lastByteAt >= 300) return response;
    delay(1);  // 让出 CPU 并喂看门狗
  }
  return response;
}

String esimCompactATResponse(const String& response) {
  String out = response;
  out.replace("\r", " ");
  out.replace("\n", " ");
  out.trim();
  while (out.indexOf("  ") >= 0) out.replace("  ", " ");
  if (out.length() > 360) out = out.substring(0, 360) + "...";
  return out;
}

static bool isIgnorableLine(const String& line) {
  return line.length() == 0 || line == "OK" || line == "ERROR" || line.startsWith("AT") ||
         line.startsWith("+CME ERROR") || line.startsWith("+CMS ERROR");
}

static bool firstDataLine(const String& response, String* payload) {
  int start = 0;
  while (start <= response.length()) {
    int end = response.indexOf('\n', start);
    if (end < 0) end = response.length();
    String line = response.substring(start, end);
    line.trim();
    if (!isIgnorableLine(line)) {
      *payload = line;
      return true;
    }
    if (end == response.length()) break;
    start = end + 1;
  }
  return false;
}

static void stripTerminator(String* line) {
  line->trim();
  if (line->endsWith("OK")) {
    line->remove(line->length() - 2);
    line->trim();
  }
  if (line->endsWith("ERROR")) {
    line->remove(line->length() - 5);
    line->trim();
  }
}

bool esimParseRequiredATPayload(const String& response, const char* prefix, String* payload) {
  if (!payload || !prefix || prefix[0] == '\0') return false;
  int index = response.indexOf(prefix);
  if (index < 0) return false;
  int start = index + strlen(prefix);
  int end = response.indexOf('\n', start);
  if (end < 0) end = response.length();
  String line = response.substring(start, end);
  line.trim();
  stripTerminator(&line);
  if (line.length() == 0) return false;
  *payload = line;
  return true;
}

bool esimParseATPayload(const String& response, const char* prefix, String* payload) {
  if (!payload) return false;
  if (esimParseRequiredATPayload(response, prefix, payload)) return true;
  if (!firstDataLine(response, payload)) return false;
  stripTerminator(payload);
  return payload->length() > 0;
}

bool esimParseCGLAHexPayload(const String& response, String* hex) {
  if (!hex) return false;
  int index = response.indexOf("+CGLA:");
  if (index >= 0) {
    int pos = index + 6;
    while (pos < response.length() && isspace((unsigned char)response.charAt(pos))) pos++;
    int expectedChars = 0;
    while (pos < response.length() && isdigit((unsigned char)response.charAt(pos))) {
      expectedChars = expectedChars * 10 + response.charAt(pos++) - '0';
    }
    int comma = response.indexOf(',', pos);
    if (comma >= 0) {
      pos = comma + 1;
      while (pos < response.length() && isspace((unsigned char)response.charAt(pos))) pos++;
      if (pos < response.length() && response.charAt(pos) == '"') pos++;
      String collected;
      if (expectedChars > 0) collected.reserve(expectedChars);
      for (; pos < response.length(); pos++) {
        char c = response.charAt(pos);
        if (isxdigit((unsigned char)c)) {
          collected += c;
          if (expectedChars > 0 && collected.length() >= expectedChars) break;
        } else if (c == '"' && expectedChars == 0) {
          break;
        }
      }
      if (collected.length() > 0) {
        if (expectedChars > 0 && collected.length() != expectedChars) {
          logCaptureLn(String("eSIM CGLA 长度字段不匹配: 期望=") + expectedChars + ", 实际=" + collected.length());
        }
        *hex = collected;
        return true;
      }
    }
  }

  String payload;
  if (!esimParseATPayload(response, "+CGLA:", &payload)) return esimExtractLongestHexRun(response, hex);
  int comma = payload.indexOf(',');
  *hex = comma >= 0 ? payload.substring(comma + 1) : payload;
  hex->trim();
  if (hex->length() >= 2 && hex->charAt(0) == '"' && hex->charAt(hex->length() - 1) == '"') {
    *hex = hex->substring(1, hex->length() - 1);
  }
  hex->trim();
  return hex->length() > 0;
}
