#ifndef ESIM_TLV_H
#define ESIM_TLV_H

#include <Arduino.h>

struct EsimTlvNode {
  uint32_t tag;
  const uint8_t* value;
  size_t length;
  size_t nextOffset;
};

bool esimIsHexString(const String& text);
bool esimHexToBytes(const String& hex, uint8_t* out, size_t outSize, size_t* outLen);
String esimBytesToHex(const uint8_t* data, size_t len);
String esimPrintableHex(const String& input);
bool esimExtractLongestHexRun(const String& input, String* hex);
void esimCopyBytesAsString(char* dst, size_t dstSize, const uint8_t* src, size_t len);
void esimBcdToIccid(char* out, size_t outSize, const uint8_t* bcd, size_t bcdLen);
bool esimIccidToBcd(const String& iccid, uint8_t* out, size_t outSize, size_t* outLen);
uint32_t esimParseInteger(const uint8_t* value, size_t len);
bool esimReadTlv(const uint8_t* data, size_t len, size_t offset, EsimTlvNode* node);
bool esimFindChildTag(const uint8_t* data, size_t len, uint32_t tag, EsimTlvNode* found);
void esimAppendTlv(uint8_t* out, size_t* pos, uint32_t tag, const uint8_t* value, size_t len);
String esimTagToHex(uint32_t tag);

#endif
