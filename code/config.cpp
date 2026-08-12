#include "config.h"
#include "logger.h"

void saveConfig() {
  preferences.begin("sms_config", false);
  preferences.putString("hubMqtt", config.smsHubMqttBroker);
  preferences.putString("hubMqttUser", config.smsHubMqttUser);
  preferences.putString("hubMqttPass", config.smsHubMqttPass);
  preferences.end();
  logCaptureLn(String("配置已保存"));
}

void loadConfig() {
  preferences.begin("sms_config", true);
  config.smsHubMqttBroker = preferences.getString("hubMqtt", "");
  config.smsHubMqttUser = preferences.getString("hubMqttUser", "");
  config.smsHubMqttPass = preferences.getString("hubMqttPass", "");
  preferences.end();
  logCaptureLn(String("配置已加载"));
}

bool isConfigValid() {
  return config.smsHubMqttBroker.length() > 0;
}
