#include "esim_tlv.h"

#include <ctype.h>

static bool isHexChar(char c) {
  return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');
}

static uint8_t hexNibble(char c) {
  if (c >= '0' && c <= '9') return c - '0';
  c = tolower(c);
  return c - 'a' + 10;
}

bool esimIsHexString(const String& text) {
  if (text.length() == 0 || (text.length() % 2) != 0) return false;
  for (int i = 0; i < text.length(); i++) {
    if (!isHexChar(text.charAt(i))) return false;
  }
  return true;
}

bool esimHexToBytes(const String& hex, uint8_t* out, size_t outSize, size_t* outLen) {
  if (!out || !outLen || !esimIsHexString(hex)) return false;
  size_t len = hex.length() / 2;
  if (len > outSize) return false;
  for (size_t i = 0; i < len; i++) out[i] = (hexNibble(hex.charAt(i * 2)) << 4) | hexNibble(hex.charAt(i * 2 + 1));
  *outLen = len;
  return true;
}

String esimBytesToHex(const uint8_t* data, size_t len) {
  static const char digits[] = "0123456789ABCDEF";
  String out;
  out.reserve(len * 2);
  for (size_t i = 0; i < len; i++) {
    out += digits[data[i] >> 4];
    out += digits[data[i] & 0x0F];
  }
  return out;
}

String esimPrintableHex(const String& input) {
  String out;
  for (int i = 0; i < input.length(); i++) if (isHexChar(input.charAt(i))) out += input.charAt(i);
  return out;
}

bool esimExtractLongestHexRun(const String& input, String* hex) {
  if (!hex) return false;
  String best;
  String current;
  for (int i = 0; i < input.length(); i++) {
    char c = input.charAt(i);
    if (isHexChar(c)) {
      current += c;
    } else {
      if ((current.length() % 2) != 0) current.remove(current.length() - 1);
      if (current.length() > best.length()) best = current;
      current = "";
    }
  }
  if ((current.length() % 2) != 0) current.remove(current.length() - 1);
  if (current.length() > best.length()) best = current;
  if (best.length() < 4) return false;
  *hex = best;
  return true;
}

void esimCopyBytesAsString(char* dst, size_t dstSize, const uint8_t* src, size_t len) {
  if (!dst || dstSize == 0) return;
  size_t count = len < dstSize - 1 ? len : dstSize - 1;
  memcpy(dst, src, count);
  dst[count] = '\0';
}

void esimBcdToIccid(char* out, size_t outSize, const uint8_t* bcd, size_t bcdLen) {
  if (!out || outSize == 0) return;
  size_t count = 0;
  for (size_t i = 0; i < bcdLen && count + 1 < outSize; i++) {
    uint8_t low = bcd[i] & 0x0F;
    uint8_t high = (bcd[i] >> 4) & 0x0F;
    if (low <= 9) out[count++] = '0' + low;
    if (high <= 9 && count + 1 < outSize) out[count++] = '0' + high;
  }
  out[count] = '\0';
}

bool esimIccidToBcd(const String& value, uint8_t* out, size_t outSize, size_t* outLen) {
  if (!out || !outLen || outSize < 10) return false;
  String digits = value;
  digits.trim();
  if (digits.length() == 0 || digits.length() > 20) return false;
  for (int i = 0; i < digits.length(); i++) if (!isdigit((unsigned char)digits.charAt(i))) return false;
  memset(out, 0xFF, 10);
  for (int i = 0; i < digits.length(); i += 2) {
    uint8_t low = digits.charAt(i) - '0';
    uint8_t high = i + 1 < digits.length() ? digits.charAt(i + 1) - '0' : 0x0F;
    out[i / 2] = (high << 4) | low;
  }
  *outLen = 10;
  return true;
}

uint32_t esimParseInteger(const uint8_t* value, size_t len) {
  uint32_t out = 0;
  for (size_t i = 0; i < len; i++) out = (out << 8) | value[i];
  return out;
}

bool esimReadTlv(const uint8_t* data, size_t len, size_t offset, EsimTlvNode* node) {
  if (!data || !node || offset >= len) return false;
  size_t pos = offset;
  uint32_t tag = data[pos++];
  if ((tag & 0x1F) == 0x1F) {
    do {
      if (pos >= len) return false;
      tag = (tag << 8) | data[pos];
    } while ((data[pos++] & 0x80) != 0);
  }
  if (pos >= len) return false;
  uint8_t lengthByte = data[pos++];
  size_t valueLength = 0;
  if ((lengthByte & 0x80) == 0) {
    valueLength = lengthByte;
  } else {
    uint8_t lengthBytes = lengthByte & 0x7F;
    if (lengthBytes == 0 || lengthBytes > sizeof(size_t) || pos + lengthBytes > len) return false;
    for (uint8_t i = 0; i < lengthBytes; i++) valueLength = (valueLength << 8) | data[pos++];
  }
  if (pos + valueLength > len) return false;
  node->tag = tag;
  node->value = data + pos;
  node->length = valueLength;
  node->nextOffset = pos + valueLength;
  return true;
}

bool esimFindChildTag(const uint8_t* data, size_t len, uint32_t tag, EsimTlvNode* found) {
  size_t pos = 0;
  EsimTlvNode node;
  while (esimReadTlv(data, len, pos, &node)) {
    if (node.tag == tag) {
      if (found) *found = node;
      return true;
    }
    pos = node.nextOffset;
  }
  return false;
}

static void appendLength(uint8_t* out, size_t* pos, size_t len) {
  if (len < 0x80) {
    out[(*pos)++] = (uint8_t)len;
  } else if (len <= 0xFF) {
    out[(*pos)++] = 0x81;
    out[(*pos)++] = (uint8_t)len;
  } else {
    out[(*pos)++] = 0x82;
    out[(*pos)++] = (uint8_t)(len >> 8);
    out[(*pos)++] = (uint8_t)len;
  }
}

static void appendTag(uint8_t* out, size_t* pos, uint32_t tag) {
  if (tag > 0xFFFF) out[(*pos)++] = (uint8_t)(tag >> 16);
  if (tag > 0xFF) out[(*pos)++] = (uint8_t)(tag >> 8);
  out[(*pos)++] = (uint8_t)tag;
}

void esimAppendTlv(uint8_t* out, size_t* pos, uint32_t tag, const uint8_t* value, size_t len) {
  appendTag(out, pos, tag);
  appendLength(out, pos, len);
  if (len > 0) {
    memcpy(out + *pos, value, len);
    *pos += len;
  }
}

String esimTagToHex(uint32_t tag) {
  String out = String(tag, HEX);
  out.toUpperCase();
  return out;
}
