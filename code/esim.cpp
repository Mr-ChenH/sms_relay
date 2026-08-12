#include "esim.h"
#include "esim_at.h"
#include "esim_es10.h"
#include "esim_internal.h"
#include "esim_tlv.h"
#include "logger.h"

static char lastError[128] = "";
static int lastProfileResult = 0;

void esimSetError(const char* message) {
  strncpy(lastError, message ? message : "", sizeof(lastError) - 1);
  lastError[sizeof(lastError) - 1] = '\0';
}

void esimSetError(const String& message) {
  esimSetError(message.c_str());
}

const char* esimGetLastError() {
  return lastError;
}

bool esimInit() {
  esimSetError("");
  String response = esimSendAT("AT", 2000);
  if (response.indexOf("OK") < 0) {
    esimSetError(String("模组 AT 无响应: ") + response);
    return false;
  }
  const char* commands[] = {"AT+CCHO=?", "AT+CCHC=?", "AT+CGLA=?"};
  for (const char* command : commands) {
    response = esimSendAT(command, 3000);
    if (response.indexOf("OK") < 0) {
      esimSetError(String("模组不支持 eUICC AT 命令 ") + command + ": " + esimCompactATResponse(response));
      return false;
    }
  }
  return true;
}

bool esimGetEID(char* eid, size_t bufferSize) {
  if (!eid || bufferSize == 0) return false;
  eid[0] = '\0';
  uint8_t request[] = {0xBF, 0x3E, 0x03, 0x5C, 0x01, 0x5A};
  uint8_t* response = nullptr;
  size_t responseLength = 0;
  if (!esimES10Command(request, sizeof(request), &response, &responseLength)) return false;

  EsimTlvNode top;
  EsimTlvNode eidNode;
  bool ok = esimReadTlv(response, responseLength, 0, &top) && top.tag == 0xBF3E &&
            esimFindChildTag(top.value, top.length, 0x5A, &eidNode);
  if (ok) {
    String value = esimBytesToHex(eidNode.value, eidNode.length);
    strncpy(eid, value.c_str(), bufferSize - 1);
    eid[bufferSize - 1] = '\0';
  } else {
    esimSetError("无法解析 EID 响应");
  }
  free(response);
  return ok;
}

int esimGetProfiles(ESimProfile* profiles, int maxProfiles) {
  if (!profiles || maxProfiles <= 0) {
    esimSetError("profile 缓冲区无效");
    return -1;
  }
  memset(profiles, 0, sizeof(ESimProfile) * maxProfiles);
  uint8_t request[] = {0xBF, 0x2D, 0x0A, 0x5C, 0x08, 0x5A, 0x4F, 0x9F, 0x70, 0x90, 0x91, 0x92, 0x95};
  uint8_t* response = nullptr;
  size_t responseLength = 0;
  if (!esimES10Command(request, sizeof(request), &response, &responseLength)) return -1;

  EsimTlvNode top;
  EsimTlvNode okNode;
  if (!esimReadTlv(response, responseLength, 0, &top) || top.tag != 0xBF2D ||
      !esimFindChildTag(top.value, top.length, 0xA0, &okNode)) {
    free(response);
    esimSetError("无法解析 profile 列表响应");
    return -1;
  }

  int count = 0;
  size_t offset = 0;
  EsimTlvNode profileNode;
  while (count < maxProfiles && esimReadTlv(okNode.value, okNode.length, offset, &profileNode)) {
    offset = profileNode.nextOffset;
    if (profileNode.tag != 0xE3 && profileNode.tag != 0xBF25) continue;
    ESimProfile& profile = profiles[count];
    profile.state = -1;
    profile.profileClass = -1;
    size_t childOffset = 0;
    EsimTlvNode child;
    while (esimReadTlv(profileNode.value, profileNode.length, childOffset, &child)) {
      childOffset = child.nextOffset;
      switch (child.tag) {
        case 0x5A:
          esimBcdToIccid(profile.iccid, sizeof(profile.iccid), child.value, child.length);
          break;
        case 0x4F: {
          String aid = esimBytesToHex(child.value, child.length);
          strncpy(profile.isdpAid, aid.c_str(), sizeof(profile.isdpAid) - 1);
          break;
        }
        case 0x9F70:
          profile.state = (int)esimParseInteger(child.value, child.length);
          break;
        case 0x90:
          esimCopyBytesAsString(profile.nickname, sizeof(profile.nickname), child.value, child.length);
          break;
        case 0x91:
          esimCopyBytesAsString(profile.serviceProviderName, sizeof(profile.serviceProviderName), child.value, child.length);
          break;
        case 0x92:
          esimCopyBytesAsString(profile.profileName, sizeof(profile.profileName), child.value, child.length);
          break;
        case 0x95:
          profile.profileClass = (int)esimParseInteger(child.value, child.length);
          break;
      }
    }
    count++;
  }
  free(response);
  return count;
}

static const char* profileOperationReason(int result, bool enable) {
  switch (result) {
    case 1: return "ICCID 或 AID 未找到";
    case 2: return enable ? "profile 不是禁用状态" : "profile 不是启用状态";
    case 3: return "被 profile 策略禁止";
    case 4: return "错误的 profile 重新启用操作";
    case 5: return "CAT busy/终端忙";
    case -1: return "内部错误，可能是 ICCID/AID 编码非法";
    default: return "未知错误";
  }
}

static bool buildProfileIdentifier(const char* text, uint8_t* output, size_t outputSize, size_t* outputLength) {
  String id = text ? text : "";
  id.trim();
  if (id.length() == 0) {
    esimSetError("ICCID/AID 不能为空");
    return false;
  }
  uint8_t bytes[16];
  size_t length = 0;
  uint32_t tag = 0x5A;
  if (id.length() == 32 && esimIsHexString(id)) {
    tag = 0x4F;
    if (!esimHexToBytes(id, bytes, sizeof(bytes), &length)) {
      esimSetError("AID HEX 编码非法");
      return false;
    }
  } else if (!esimIccidToBcd(id, bytes, sizeof(bytes), &length)) {
    esimSetError("ICCID 编码非法");
    return false;
  }
  *outputLength = 0;
  esimAppendTlv(output, outputLength, tag, bytes, length);
  return *outputLength <= outputSize;
}

static bool profileOperation(uint32_t outerTag, const char* id, bool refresh, bool enableReason, bool refreshInChoice = false) {
  lastProfileResult = 0;
  uint8_t identifier[24];
  size_t identifierLength = 0;
  if (!buildProfileIdentifier(id, identifier, sizeof(identifier), &identifierLength)) return false;

  uint8_t request[40];
  size_t requestLength = 0;
  if (outerTag == 0xBF33) {
    esimAppendTlv(request, &requestLength, outerTag, identifier, identifierLength);
  } else {
    uint8_t refreshValue = refresh ? 0xFF : 0x00;
    uint8_t value[32];
    size_t valueLength = 0;
    if (refreshInChoice) {
      uint8_t choice[28];
      size_t choiceLength = 0;
      memcpy(choice, identifier, identifierLength);
      choiceLength += identifierLength;
      esimAppendTlv(choice, &choiceLength, 0x81, &refreshValue, 1);
      esimAppendTlv(value, &valueLength, 0xA0, choice, choiceLength);
    } else {
      esimAppendTlv(value, &valueLength, 0xA0, identifier, identifierLength);
      esimAppendTlv(value, &valueLength, 0x81, &refreshValue, 1);
    }
    esimAppendTlv(request, &requestLength, outerTag, value, valueLength);
  }

  uint8_t* response = nullptr;
  size_t responseLength = 0;
  if (!esimES10Command(request, requestLength, &response, &responseLength)) return false;
  EsimTlvNode top;
  EsimTlvNode resultNode;
  bool parsed = esimReadTlv(response, responseLength, 0, &top) && top.tag == outerTag &&
                esimFindChildTag(top.value, top.length, 0x80, &resultNode);
  int result = parsed ? (int)esimParseInteger(resultNode.value, resultNode.length) : -1;
  lastProfileResult = result;
  free(response);
  if (!parsed) {
    esimSetError("无法解析 profile 操作响应");
    return false;
  }
  if (result != 0) {
    esimSetError(profileOperationReason(result, enableReason));
    return false;
  }
  return true;
}

bool esimEnableProfile(const char* id) { return profileOperation(0xBF31, id, true, true); }
bool esimDisableProfile(const char* id) { return profileOperation(0xBF32, id, true, false); }
bool esimDeleteProfile(const char* id) { return profileOperation(0xBF33, id, false, false); }

bool esimSwitchProfile(const char* id) {
  if (profileOperation(0xBF31, id, true, true)) return true;
  if (lastProfileResult == 5 && profileOperation(0xBF31, id, false, true)) {
    logCaptureLn("eSIM 无刷新切换成功，可能需要重启模组后生效");
    return true;
  }
  if (lastProfileResult == 5 && profileOperation(0xBF31, id, false, true, true)) {
    logCaptureLn("eSIM lpac 兼容格式切换成功，可能需要重启模组后生效");
    return true;
  }
  return false;
}

bool esimGetNotificationCount(int* count) {
  if (count) *count = 0;
  esimSetError("通知查询暂未实现");
  return false;
}
