#ifndef ESIM_H
#define ESIM_H

#include "globals.h"

#define ESIM_MAX_ICCID_LEN 21
#define ESIM_MAX_ISDP_AID_LEN 33
#define ESIM_MAX_NAME_LEN 64

#ifndef ESIM_PROFILE_LOG
#define ESIM_PROFILE_LOG 0
#endif

struct ESimProfile {
  char iccid[ESIM_MAX_ICCID_LEN];
  char isdpAid[ESIM_MAX_ISDP_AID_LEN];
  char nickname[ESIM_MAX_NAME_LEN];
  char serviceProviderName[ESIM_MAX_NAME_LEN];
  char profileName[ESIM_MAX_NAME_LEN];
  int state;
  int profileClass;
};

struct ESimInfo {
  char profileVersion[16];
  char svn[16];
  char firmwareVersion[16];
  char globalPlatformVersion[16];
  char category[24];
  char sasAccreditationNumber[64];
  uint32_t installedApplications;
  uint32_t freeNonVolatileMemory;
  uint32_t freeVolatileMemory;
};

// eUICC 是否初始化成功（esimInit）。未就绪时 esimGetProfiles/esimGetEID
// 立即失败返回，避免身份刷新等路径被慢速 APDU 查询阻塞。
extern bool esimReady;

bool esimInit();
bool esimGetEID(char* eid, size_t bufferSize);
bool esimGetInfo(ESimInfo* info);
int esimGetProfiles(ESimProfile* profiles, int maxProfiles);
bool esimEnableProfile(const char* iccidOrAid);
bool esimDisableProfile(const char* iccidOrAid);
bool esimDeleteProfile(const char* iccidOrAid);
bool esimSwitchProfile(const char* iccidOrAid);
bool esimGetNotificationCount(int* count);
const char* esimGetLastError();

bool handleSerialConsole();
bool handleESimSerialCommand(const String& command);

#endif
