#include "esim.h"
#include "wifi_manager.h"

static void printProfile(const ESimProfile& profile, int index) {
  Serial.print(index);
  Serial.print(". ICCID: ");
  Serial.print(profile.iccid[0] ? profile.iccid : "-");
  Serial.print(" | 状态: ");
  Serial.print(profile.state == 1 ? "已启用" : profile.state == 0 ? "已禁用" : "未知");
  Serial.print(" | 名称: ");
  Serial.print(profile.nickname[0] ? profile.nickname : profile.profileName[0] ? profile.profileName : "-");
  if (profile.serviceProviderName[0]) {
    Serial.print(" | 运营商: ");
    Serial.print(profile.serviceProviderName);
  }
  Serial.println();
}

bool handleESimSerialCommand(const String& command) {
  String line = command;
  line.trim();
  if (!line.startsWith("esim")) return false;
  String args = line.substring(4);
  args.trim();
  if (args.length() == 0 || args == "help") {
    Serial.println("eSIM 命令: list, eid, info, switch/enable/disable <iccid|aid>");
    return true;
  }
  if (args == "list") {
    ESimProfile profiles[10];
    int count = esimGetProfiles(profiles, 10);
    if (count < 0) Serial.println(esimGetLastError());
    else for (int i = 0; i < count; i++) printProfile(profiles[i], i + 1);
    return true;
  }
  if (args == "eid") {
    char eid[40];
    if (esimGetEID(eid, sizeof(eid))) Serial.println(eid);
    else Serial.println(esimGetLastError());
    return true;
  }
  if (args == "info") {
    ESimInfo info;
    if (!esimGetInfo(&info)) {
      Serial.println(esimGetLastError());
      return true;
    }
    Serial.println(String("Profile package: ") + (info.profileVersion[0] ? info.profileVersion : "-"));
    Serial.println(String("SGP.22 SVN: ") + (info.svn[0] ? info.svn : "-"));
    Serial.println(String("Firmware: ") + (info.firmwareVersion[0] ? info.firmwareVersion : "-"));
    Serial.println(String("GlobalPlatform: ") + (info.globalPlatformVersion[0] ? info.globalPlatformVersion : "-"));
    Serial.println(String("Category: ") + (info.category[0] ? info.category : "-"));
    Serial.println(String("SAS: ") + (info.sasAccreditationNumber[0] ? info.sasAccreditationNumber : "-"));
    Serial.println(String("Installed applications: ") + info.installedApplications);
    Serial.println(String("Free NVM: ") + info.freeNonVolatileMemory + " bytes");
    Serial.println(String("Free volatile memory: ") + info.freeVolatileMemory + " bytes");
    Serial.println("Total NVM: not reported by EUICCInfo2");
    return true;
  }
  int space = args.indexOf(' ');
  String action = space >= 0 ? args.substring(0, space) : args;
  String id = space >= 0 ? args.substring(space + 1) : "";
  id.trim();
  if (id.length() == 0) {
    Serial.println("缺少 ICCID/AID 参数");
    return true;
  }
  bool ok = false;
  if (action == "switch" || action == "enable") ok = esimSwitchProfile(id.c_str());
  else if (action == "disable") ok = esimDisableProfile(id.c_str());
  else {
    Serial.println("未知 eSIM 命令");
    return true;
  }
  Serial.println(ok ? "操作成功" : esimGetLastError());
  return true;
}

bool handleSerialConsole() {
  static String line;
  bool consumed = false;
  while (Serial.available()) {
    char c = (char)Serial.read();
    consumed = true;
    if (c == 0x1A) {
      Serial1.write(0x1A);
      line = "";
    } else if (c == '\r' || c == '\n') {
      if (line.length() > 0) {
        String command = line;
        line = "";
        if (!handleESimSerialCommand(command) && !provisionSerialCommand(command)) Serial1.println(command);
      }
    } else if (line.length() < 160) {
      line += c;
    }
    delay(1);  // 长输入流（如粘贴）时让出 CPU 并喂看门狗，避免长时间占用主循环
  }
  return consumed;
}
