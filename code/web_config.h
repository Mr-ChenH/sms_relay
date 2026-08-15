#ifndef WEB_CONFIG_H
#define WEB_CONFIG_H

#include "globals.h"

// SoftAP 配网 Web 服务器：
// 设备开启热点 SMSHub-XXXXXX（无密码），手机连接后访问 http://192.168.4.1
// 选择 WiFi、填写密码与 SMS Hub 服务器地址完成配网。
// 配网数据只暂存，由主循环 wifiManagerLoop -> applyPendingConfiguration 统一写入 NVS。

void webConfigInit();
void webConfigLoop();

#endif
