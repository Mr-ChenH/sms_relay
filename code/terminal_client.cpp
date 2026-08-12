#include "terminal_client.h"
#include "wifi_config.h"
#include "config.h"
#include "logger.h"
#include "modem.h"
#include "esim.h"
#include "esim_at.h"
#include "esim_tlv.h"
#include "sms_queue.h"
#include <ArduinoJson.h>
#include <WiFiClient.h>
#include <PubSubClient.h>

#ifndef SMS_HUB_DEFAULT_API_BASE_URL
#define SMS_HUB_DEFAULT_API_BASE_URL ""
#endif

#ifndef SMS_HUB_DEVICE_ID
#define SMS_HUB_DEVICE_ID ""
#endif

static const unsigned long REGISTER_INTERVAL_MS = 300000;
static const unsigned long HEARTBEAT_INTERVAL_MS = 5000;
static const unsigned long LOG_FLUSH_INTERVAL_MS = 15000;
static const unsigned long MQTT_RECONNECT_INTERVAL_MS = 5000;
static const unsigned long ESIM_PROFILE_SYNC_RETRY_MS = 5000;
static const uint16_t MQTT_KEEPALIVE_SECONDS = 20;
static const unsigned long ESIM_PROFILE_SYNC_INTERVAL_MS = 300000;
static const unsigned long IDENTITY_REFRESH_INTERVAL_MS = 300000;
static const unsigned long IDENTITY_SETTLE_INTERVAL_MS = 10000;
static const unsigned long IDENTITY_SETTLE_WINDOW_MS = 120000;
static const size_t MAX_LOG_QUEUE = 8;

static unsigned long smsSequence = 0;

static WiFiClient mqttWifiClient;
static PubSubClient mqttClient(mqttWifiClient);
static bool mqttConfigured = false;
static bool commandExecuting = false;
static String pendingCommandPayload;
static String pendingApduPayload;
static bool apduSessionConnected = false;
static int apduLogicalChannel = 0;
static String activeCommandId;
static String lastCompletedCommandId;
static String pendingResultCommandId;
static String pendingResultBody;
static bool pendingResultOK = false;
static String activeMqttBroker;
static String activeMqttUser;
static String activeMqttPass;
static unsigned long lastMqttReconnectAt = 0;
static unsigned long lastMqttErrorLogAt = 0;
static int lastMqttState = 0;

static String terminalDeviceID();

static String cachedICCID;
static String cachedEID;
static String cachedOperator;
static String cachedPhoneNumber;
static unsigned long lastIdentityRefreshAt = 0;
static unsigned long identitySettleUntil = 0;

static bool registered = false;
static bool esimProfileSyncPending = false;
static unsigned long lastRegisterAt = 0;
static unsigned long lastHeartbeatAt = 0;
static unsigned long lastLogFlushAt = 0;
static unsigned long lastEsimProfileSyncAt = 0;
static String logQueue[MAX_LOG_QUEUE];
static size_t logQueueSize = 0;

static void flushSMSQueue();

static String mqttBrokerURL() {
  String broker = config.smsHubMqttBroker;
  broker.trim();
  return broker;
}

static bool mqttEnabled() {
  return mqttBrokerURL().length() > 0;
}

static String mqttBaseTopic() {
  return "sms-hub/devices/" + terminalDeviceID();
}

static bool parseMqttBroker(String broker, String& host, uint16_t& port) {
  broker.trim();
  broker.replace("mqtt://", "");
  int slash = broker.indexOf('/');
  if (slash >= 0) broker = broker.substring(0, slash);
  int colon = broker.lastIndexOf(':');
  port = 1883;
  if (colon > 0) {
    port = (uint16_t)broker.substring(colon + 1).toInt();
    host = broker.substring(0, colon);
  } else {
    host = broker;
  }
  host.trim();
  return host.length() > 0 && port > 0;
}

static bool publishJSON(const String& topicSuffix, JsonDocument& doc, bool retained = false) {
  if (!mqttClient.connected()) return false;
  String body;
  serializeJson(doc, body);
  return mqttClient.publish((mqttBaseTopic() + topicSuffix).c_str(), body.c_str(), retained);
}

static bool publishText(const String& topicSuffix, const String& body, bool retained = false) {
  if (!mqttClient.connected()) return false;
  return mqttClient.publish((mqttBaseTopic() + topicSuffix).c_str(), body.c_str(), retained);
}

bool terminalClientEnabled() {
  return mqttEnabled();
}

static String terminalDeviceID() {
  String configured = String(SMS_HUB_DEVICE_ID);
  configured.trim();
  if (configured.length() > 0) return configured;
  String mac = WiFi.macAddress();
  mac.replace(":", "");
  mac.toLowerCase();
  return "esp32-" + mac;
}

static String currentTimestampISO() {
  time_t now = time(nullptr);
  if (now < 100000) return "";
  struct tm timeInfo;
  gmtime_r(&now, &timeInfo);
  char buf[25];
  strftime(buf, sizeof(buf), "%Y-%m-%dT%H:%M:%SZ", &timeInfo);
  return String(buf);
}

static String firstQuotedValue(const String& text, int startAt = 0) {
  int start = text.indexOf('"', startAt);
  if (start < 0) return "";
  int end = text.indexOf('"', start + 1);
  if (end < 0) return "";
  return text.substring(start + 1, end);
}

static String parseOperatorName(const String& resp) {
  int idx = resp.indexOf("+COPS:");
  if (idx < 0) return "";
  String name = firstQuotedValue(resp, idx);
  name.trim();
  return name;
}

static String parsePhoneNumber(const String& resp) {
  int idx = resp.indexOf("+CNUM:");
  if (idx < 0) return "";
  int firstQuote = resp.indexOf('"', idx);
  if (firstQuote < 0) return "";
  int secondQuote = resp.indexOf('"', firstQuote + 1);
  if (secondQuote < 0) return "";
  String number = firstQuotedValue(resp, secondQuote + 1);
  number.trim();
  return number;
}

static String parseMCCID(const String& resp) {
  int idx = resp.indexOf("+MCCID:");
  if (idx < 0) return "";
  int start = idx + 7;
  int end = resp.indexOf('\n', start);
  String value = end > start ? resp.substring(start, end) : resp.substring(start);
  value.replace("\r", "");
  value.trim();
  return value;
}

static void refreshIdentityCache(bool force = false) {
  unsigned long now = millis();
  unsigned long interval = identitySettleUntil > 0 && (long)(identitySettleUntil - now) > 0
                             ? IDENTITY_SETTLE_INTERVAL_MS
                             : IDENTITY_REFRESH_INTERVAL_MS;
  if (!force && lastIdentityRefreshAt > 0 && now - lastIdentityRefreshAt < interval) return;
  lastIdentityRefreshAt = now;

  String activeICCID;
  String profileProvider;
  ESimProfile profiles[10];
  int count = esimGetProfiles(profiles, 10);
  if (count >= 0) {
    for (int i = 0; i < count; i++) {
      if (profiles[i].state == 1 && profiles[i].iccid[0]) {
        activeICCID = profiles[i].iccid;
        profileProvider = profiles[i].serviceProviderName;
        break;
      }
    }
  }
  if (activeICCID.length() == 0) activeICCID = parseMCCID(sendATCommand("AT+MCCID", 3000));
  cachedICCID = activeICCID;

  char eid[40];
  if (esimGetEID(eid, sizeof(eid))) cachedEID = eid;

  String op = parseOperatorName(sendATCommand("AT+COPS?", 5000));
  cachedOperator = op.length() > 0 ? op : profileProvider;

  cachedPhoneNumber = parsePhoneNumber(sendATCommand("AT+CNUM", 3000));
}

static void terminalRegister() {
  if (!terminalClientEnabled()) return;
  JsonDocument doc;
  doc["deviceId"] = terminalDeviceID();
  doc["name"] = terminalDeviceID();
  doc["firmwareVersion"] = "0.9.0-terminal";
  doc["hardwareModel"] = "ESP32-C3 + ML307A";
  if (publishJSON("/register", doc, false)) {
    registered = true;
    terminalReportLog("info", "terminal registered");
  }
  lastRegisterAt = millis();
}

static void terminalHeartbeat(bool refreshIdentity = true) {
  if (refreshIdentity) refreshIdentityCache();
  JsonDocument doc;
  doc["deviceId"] = terminalDeviceID();
  doc["firmwareVersion"] = "0.9.0-terminal";
  doc["hardwareModel"] = "ESP32-C3 + ML307A";
  doc["operator"] = cachedOperator;
  doc["iccid"] = cachedICCID;
  doc["eid"] = cachedEID;
  doc["phoneNumber"] = cachedPhoneNumber;
  doc["rssi"] = WiFi.RSSI();
  doc["freeHeapKb"] = ESP.getFreeHeap() / 1024;
  doc["uptime"] = String(millis() / 1000) + "s";
  if (publishJSON("/heartbeat", doc, false)) lastHeartbeatAt = millis();
}

void terminalClientIdentityReady() {
  cachedICCID = "";
  cachedEID = "";
  cachedOperator = "";
  cachedPhoneNumber = "";
  lastIdentityRefreshAt = 0;
  identitySettleUntil = millis() + IDENTITY_SETTLE_WINDOW_MS;
  refreshIdentityCache(true);
  terminalSyncEsimProfiles();
  terminalHeartbeat(false);
  terminalReportLog("info", String("initial identity reported: iccid=") + cachedICCID +
                            ", operator=" + cachedOperator + ", phone=" + cachedPhoneNumber);
}

static void refreshIdentityAfterProfileChange() {
  cachedICCID = "";
  cachedOperator = "";
  cachedPhoneNumber = "";
  lastIdentityRefreshAt = 0;
  identitySettleUntil = millis() + IDENTITY_SETTLE_WINDOW_MS;
  refreshIdentityCache(true);
  terminalHeartbeat(false);
  terminalReportLog("info", String("identity refreshed after profile change: iccid=") + cachedICCID +
                            ", operator=" + cachedOperator + ", phone=" + cachedPhoneNumber);
}

static void flushSMSQueue() {
  while (smsQueueSize() > 0 && mqttClient.connected()) {
    const QueuedSMS* item = smsQueueFront();
    if (!item) return;
    JsonDocument doc;
    doc["deviceId"] = terminalDeviceID();
    doc["terminalMessageId"] = item->messageId;
    doc["sender"] = item->sender;
    doc["recipient"] = item->recipient;
    doc["body"] = item->body;
    doc["timestamp"] = item->timestamp;
    doc["concatInfo"] = "1/1";
    if (!publishJSON("/sms", doc, false)) return;
    smsQueuePop();
  }
}

void terminalReportSMS(const char* sender, const char* text, const char* timestamp) {
  if (!terminalClientEnabled()) return;
  QueuedSMS item;
  item.messageId = terminalDeviceID() + "-" + String(millis()) + "-" + String(++smsSequence);
  item.sender = sender ? sender : "";
  item.recipient = cachedPhoneNumber;
  item.body = text ? text : "";
  String ts = currentTimestampISO();
  item.timestamp = ts.length() > 0 ? ts : String(timestamp ? timestamp : "");
  if (!smsQueuePush(item)) {
    terminalReportLog("error", "persistent SMS queue full; rejecting new message");
    smsQueueFlushNow();
    return;
  }
  flushSMSQueue();
}

void terminalReportLog(const String& level, const String& message) {
  if (!terminalClientEnabled()) return;
  if (logQueueSize >= MAX_LOG_QUEUE) {
    for (size_t i = 1; i < logQueueSize; i++) logQueue[i - 1] = logQueue[i];
    logQueueSize--;
  }
  logQueue[logQueueSize++] = level + "|" + message;
}

static void flushLogs() {
  if (logQueueSize == 0) return;
  JsonDocument doc;
  doc["deviceId"] = terminalDeviceID();
  JsonArray logs = doc["logs"].to<JsonArray>();
  for (size_t i = 0; i < logQueueSize; i++) {
    String item = logQueue[i];
    int sep = item.indexOf('|');
    JsonObject row = logs.add<JsonObject>();
    row["level"] = sep > 0 ? item.substring(0, sep) : "info";
    row["message"] = sep > 0 ? item.substring(sep + 1) : item;
  }
  if (publishJSON("/logs", doc, false)) logQueueSize = 0;
  lastLogFlushAt = millis();
}

static String profileStateName(int state) {
  if (state == 1) return "enabled";
  if (state == 0) return "disabled";
  return "unknown";
}

void terminalSyncEsimProfiles() {
  ESimProfile profiles[10];
  int count = esimGetProfiles(profiles, 10);
  if (count < 0) {
    esimProfileSyncPending = true;
    terminalReportLog("warn", String("esim profile sync failed: ") + esimGetLastError());
    lastEsimProfileSyncAt = millis();
    return;
  }

  JsonDocument doc;
  doc["deviceId"] = terminalDeviceID();
  JsonArray items = doc["profiles"].to<JsonArray>();
  for (int i = 0; i < count; i++) {
    JsonObject item = items.add<JsonObject>();
    item["iccid"] = profiles[i].iccid;
    item["aid"] = profiles[i].isdpAid;
    item["nickname"] = profiles[i].nickname;
    item["provider"] = profiles[i].serviceProviderName;
    item["profileName"] = profiles[i].profileName;
    item["state"] = profileStateName(profiles[i].state);
    if (profiles[i].state == 1 && profiles[i].iccid[0]) cachedICCID = profiles[i].iccid;
  }
  if (publishJSON("/esim/profiles", doc, false)) {
    esimProfileSyncPending = false;
    terminalReportLog("info", String("esim profiles synced count=") + String(count));
  } else {
    esimProfileSyncPending = true;
  }
  lastEsimProfileSyncAt = millis();
}

static String commandString(JsonVariantConst payload, const char* key) {
  if (!payload.is<JsonObjectConst>()) return "";
  const char* value = payload[key] | "";
  return String(value);
}

static bool profileIsEnabled(const String& id, String& detail) {
  ESimProfile profiles[10];
  int count = esimGetProfiles(profiles, 10);
  if (count < 0) {
    detail = String("profile verification failed: ") + esimGetLastError();
    return false;
  }
  for (int i = 0; i < count; i++) {
    if (id == profiles[i].iccid || id == profiles[i].isdpAid) {
      detail = String("profile state=") + String(profiles[i].state);
      return profiles[i].state == 1;
    }
  }
  detail = "profile not found after switch";
  return false;
}

static bool executeCommand(const String& type, JsonVariantConst payload, String& result) {
  if (type == "send_sms") {
    String phone = commandString(payload, "phone");
    String body = commandString(payload, "body");
    if (phone.length() == 0 || body.length() == 0) {
      result = "missing phone/body";
      return false;
    }
    bool ok = sendSMS(phone.c_str(), body.c_str());
    result = ok ? "sms sent" : "sms send failed";
    return ok;
  }
  if (type == "at_command") {
    String command = commandString(payload, "command");
    if (command.length() == 0) command = "AT";
    result = sendATCommand(command.c_str(), 5000);
    return result.indexOf("ERROR") < 0;
  }
  if (type == "query_signal") {
    result = sendATCommand("AT+CSQ", 3000);
    return result.indexOf("ERROR") < 0;
  }
  if (type == "query_sim") {
    result = sendATCommand("AT+CIMI", 3000) + "\n" + sendATCommand("AT+ICCID", 3000);
    return result.indexOf("ERROR") < 0;
  }
  if (type == "query_network") {
    result = sendATCommand("AT+CEREG?", 3000) + "\n" + sendATCommand("AT+COPS?", 5000);
    return result.indexOf("ERROR") < 0;
  }
  if (type == "ping") {
    String host = commandString(payload, "host");
    if (host.length() == 0) host = "8.8.8.8";
    result = sendATCommand((String("AT+MPING=\"") + host + "\",30,1").c_str(), 10000);
    return result.indexOf("ERROR") < 0;
  }
  if (type == "modem_hardreset") {
    bool ok = resetModule();
    result = ok ? "modem hard reset complete" : "modem hard reset complete but initialization timed out";
    return ok;
  }
  if (type == "modem_airplane_toggle") {
    result = sendATCommand("AT+CFUN?", 3000);
    bool airplane = result.indexOf("+CFUN: 4") >= 0;
    String setResp = sendATCommand(airplane ? "AT+CFUN=1" : "AT+CFUN=4", 5000);
    result += "\n" + setResp;
    return setResp.indexOf("ERROR") < 0;
  }
  if (type == "esim_enable_profile") {
    String id = commandString(payload, "iccid");
    terminalReportLog("info", String("eSIM profile switch started: ") + id);
    bool ok = esimSwitchProfile(id.c_str());
    if (!ok) {
      String enableError = esimGetLastError();
      if (enableError.indexOf("不是禁用状态") >= 0) {
        String alreadyEnabledDetail;
        if (profileIsEnabled(id, alreadyEnabledDetail)) {
          terminalSyncEsimProfiles();
          result = "profile already enabled and verified";
          return true;
        }
      }
      result = String("profile enable failed: ") + enableError;
      return false;
    }

    terminalReportLog("info", String("eSIM profile switch accepted: ") + id + "; verifying");
    delay(2000);
    String verifyDetail;
    if (!profileIsEnabled(id, verifyDetail)) {
      terminalReportLog("warn", String("eSIM switch accepted but not active: ") + id + ", " + verifyDetail + "; resetting modem");
      bool modemRecovered = resetModule();
      terminalReportLog(modemRecovered ? "info" : "warn", String("eSIM modem reset complete: ") + id + (modemRecovered ? "; checking eUICC" : "; network initialization timed out, checking eUICC anyway"));
      if (!esimInit()) {
        result = String("profile switch accepted, but modem reset eSIM init failed: ") + esimGetLastError();
        return false;
      }
      delay(2000);
      if (!profileIsEnabled(id, verifyDetail)) {
        terminalSyncEsimProfiles();
        result = String("profile switch accepted but target is not enabled after modem reset: ") + verifyDetail;
        return false;
      }
    }

    terminalSyncEsimProfiles();
    refreshIdentityAfterProfileChange();
    terminalReportLog("info", String("eSIM profile switch verified: ") + id);
    result = "profile enabled and verified";
    return true;
  }
  if (type == "esim_delete_profile") {
    String id = commandString(payload, "iccid");
    bool ok = esimDeleteProfile(id.c_str());
    result = ok ? "profile deleted" : String("profile delete failed: ") + esimGetLastError();
    if (ok) terminalSyncEsimProfiles();
    return ok;
  }
  if (type == "esim_download_profile") {
    result = "profile downloads are executed by the server LPA APDU session";
    return false;
  }
  result = "unsupported command type: " + type;
  return false;
}

static bool publishCommandStatus(const String& commandId, const String& status) {
  JsonDocument doc;
  doc["deviceId"] = terminalDeviceID();
  doc["status"] = status;
  return publishJSON("/commands/" + commandId + "/status", doc, false);
}

static bool publishCommandResult(const String& commandId, bool ok, const String& result) {
  JsonDocument doc;
  doc["deviceId"] = terminalDeviceID();
  doc["status"] = ok ? "succeeded" : "failed";
  doc["result"] = result;
  return publishJSON("/commands/" + commandId + "/result", doc, false);
}

static void queueCommandResult(const String& commandId, bool ok, const String& result) {
  pendingResultCommandId = commandId;
  pendingResultOK = ok;
  pendingResultBody = result;
}

static void flushCommandResult() {
  if (pendingResultCommandId.length() == 0 || !mqttClient.connected()) return;
  if (publishCommandResult(pendingResultCommandId, pendingResultOK, pendingResultBody)) {
    lastCompletedCommandId = pendingResultCommandId;
    pendingResultCommandId = "";
    pendingResultBody = "";
  }
}

static void handleApduPayload(const String& payloadText) {
  JsonDocument request;
  JsonDocument response;
  DeserializationError jsonError = deserializeJson(request, payloadText);
  String requestId = request["requestId"] | "";
  String function = request["func"] | "";
  String param = request["param"] | "";
  response["requestId"] = requestId;
  response["ecode"] = -1;

  if (jsonError || requestId.length() == 0 || function.length() == 0) {
    response["error"] = "invalid APDU request";
    publishJSON("/esim/apdu/response", response, false);
    return;
  }

  if (function == "connect") {
    apduSessionConnected = true;
    response["ecode"] = 0;
  } else if (function == "disconnect") {
    for (int channel = 1; channel <= 8; channel++) {
      esimSendAT((String("AT+CCHC=") + channel).c_str(), 2000);
    }
    apduSessionConnected = false;
    apduLogicalChannel = 0;
    response["ecode"] = 0;
    publishJSON("/esim/apdu/response", response, false);
    terminalSyncEsimProfiles();
    return;
  } else if (!apduSessionConnected) {
    response["error"] = "APDU session is not connected";
  } else if (function == "logic_channel_open") {
    if (!esimIsHexString(param) || param.length() == 0 || param.length() > 64) {
      response["error"] = "invalid AID";
    } else {
      String atResponse = esimSendAT((String("AT+CCHO=\"") + param + "\"").c_str(), 10000);
      String channelText;
      if (esimParseATPayload(atResponse, "+CCHO:", &channelText)) {
        int channel = channelText.toInt();
        apduLogicalChannel = channel > 0 ? channel : 0;
        response["ecode"] = channel > 0 ? channel : -1;
        if (channel <= 0) response["error"] = "invalid logical channel";
      } else {
        response["error"] = esimCompactATResponse(atResponse);
      }
    }
  } else if (function == "logic_channel_close") {
    if (!esimIsHexString(param) || param.length() != 2) {
      response["error"] = "invalid logical channel";
    } else {
      int channel = (int)strtol(param.c_str(), nullptr, 16);
      String atResponse = esimSendAT((String("AT+CCHC=") + channel).c_str(), 5000);
      bool ok = atResponse.indexOf("OK") >= 0;
      if (ok && channel == apduLogicalChannel) apduLogicalChannel = 0;
      response["ecode"] = ok ? 0 : -1;
      if (!ok) response["error"] = esimCompactATResponse(atResponse);
    }
  } else if (function == "transmit") {
    if (apduLogicalChannel <= 0) {
      response["error"] = "logical channel is not open";
    } else if (!esimIsHexString(param) || param.length() < 8 || param.length() > 600) {
      response["error"] = "invalid or oversized APDU";
    } else {
      String command = "AT+CGLA=" + String(apduLogicalChannel) + "," + String(param.length()) + ",\"" + param + "\"";
      String atResponse = esimSendAT(command.c_str(), 30000);
      String responseHex;
      if (esimParseCGLAHexPayload(atResponse, &responseHex) && esimIsHexString(responseHex)) {
        response["ecode"] = 0;
        response["data"] = responseHex;
      } else {
        response["error"] = esimCompactATResponse(atResponse);
      }
    }
  } else {
    response["error"] = "unsupported APDU function";
  }
  publishJSON("/esim/apdu/response", response, false);
}

static void handleCommandPayload(const String& payloadText) {
  JsonDocument doc;
  DeserializationError err = deserializeJson(doc, payloadText);
  if (err) return;
  String commandId = doc["id"] | "";
  String type = doc["type"] | "";
  if (commandId.length() == 0 || type.length() == 0 || commandId == lastCompletedCommandId) return;
  activeCommandId = commandId;
  publishCommandStatus(commandId, "claimed");
  String result;
  commandExecuting = true;
  bool ok = executeCommand(type, doc["payload"], result);
  commandExecuting = false;
  queueCommandResult(commandId, ok, result);
  terminalClientService();
  flushCommandResult();
  activeCommandId = "";
}

static void mqttCallback(char* topic, byte* payload, unsigned int length) {
  String topicText = String(topic);
  String incoming;
  incoming.reserve(length + 1);
  for (unsigned int i = 0; i < length; i++) incoming += (char)payload[i];
  if (topicText == mqttBaseTopic() + "/esim/apdu/request") {
    if (pendingApduPayload.length() == 0) pendingApduPayload = incoming;
    return;
  }
  if (topicText != mqttBaseTopic() + "/commands" || commandExecuting || pendingCommandPayload.length() > 0) return;
  if ((activeCommandId.length() > 0 && incoming.indexOf(String("\"id\":\"") + activeCommandId + "\"") >= 0) ||
      (pendingResultCommandId.length() > 0 && incoming.indexOf(String("\"id\":\"") + pendingResultCommandId + "\"") >= 0) ||
      (lastCompletedCommandId.length() > 0 && incoming.indexOf(String("\"id\":\"") + lastCompletedCommandId + "\"") >= 0)) return;
  pendingCommandPayload = incoming;
}

static void executePendingCommand() {
  if (pendingApduPayload.length() > 0 && !commandExecuting) {
    String payload = pendingApduPayload;
    pendingApduPayload = "";
    handleApduPayload(payload);
  }
  if (pendingCommandPayload.length() == 0 || commandExecuting || apduSessionConnected) return;
  String payload = pendingCommandPayload;
  pendingCommandPayload = "";
  handleCommandPayload(payload);
}

static bool ensureMqttConnected() {
  if (!mqttEnabled() || WiFi.status() != WL_CONNECTED) return false;
  if (mqttClient.connected() && activeMqttBroker == mqttBrokerURL() &&
      activeMqttUser == config.smsHubMqttUser && activeMqttPass == config.smsHubMqttPass) return true;
  if (mqttClient.connected()) mqttClient.disconnect();
  mqttWifiClient.stop();
  unsigned long now = millis();
  if (lastMqttReconnectAt > 0 && now - lastMqttReconnectAt < MQTT_RECONNECT_INTERVAL_MS) return false;
  lastMqttReconnectAt = now;

  String host;
  uint16_t port;
  if (!parseMqttBroker(mqttBrokerURL(), host, port)) return false;
  mqttClient.setServer(host.c_str(), port);
  mqttClient.setCallback(mqttCallback);
  mqttClient.setBufferSize(4096);
  mqttClient.setKeepAlive(MQTT_KEEPALIVE_SECONDS);
  mqttClient.setSocketTimeout(5);

  String clientId = terminalDeviceID();
  String willTopic = mqttBaseTopic() + "/status";
  bool ok;
  if (config.smsHubMqttUser.length() > 0) {
    ok = mqttClient.connect(clientId.c_str(), config.smsHubMqttUser.c_str(), config.smsHubMqttPass.c_str(), willTopic.c_str(), 1, true, "offline");
  } else {
    ok = mqttClient.connect(clientId.c_str(), willTopic.c_str(), 1, true, "offline");
  }
  if (!ok) {
    int state = mqttClient.state();
    if (state != lastMqttState || lastMqttErrorLogAt == 0 || now - lastMqttErrorLogAt >= 30000) {
      logCaptureLn(String("MQTT 连接失败 state=") + String(state) + ", broker=" + host + ":" + String(port));
      lastMqttState = state;
      lastMqttErrorLogAt = now;
    }
    return false;
  }
  lastMqttState = 0;
  lastMqttErrorLogAt = 0;
  activeMqttBroker = mqttBrokerURL();
  activeMqttUser = config.smsHubMqttUser;
  activeMqttPass = config.smsHubMqttPass;
  registered = false;
  esimProfileSyncPending = true;
  logCaptureLn(String("MQTT 已连接: ") + host + ":" + String(port));
  if (apduSessionConnected || apduLogicalChannel > 0) {
    if (apduLogicalChannel > 0) esimSendAT((String("AT+CCHC=") + apduLogicalChannel).c_str(), 5000);
    apduSessionConnected = false;
    apduLogicalChannel = 0;
    pendingApduPayload = "";
    terminalReportLog("warn", "abandoned eSIM APDU session cleared after MQTT reconnect");
  }
  mqttClient.subscribe((mqttBaseTopic() + "/commands").c_str(), 1);
  mqttClient.subscribe((mqttBaseTopic() + "/esim/apdu/request").c_str(), 1);
  publishText("/status", "online", true);
  terminalRegister();
  terminalHeartbeat();
  return true;
}

void terminalClientConfigChanged() {
  if (mqttClient.connected()) mqttClient.disconnect();
  mqttWifiClient.stop();
  activeMqttBroker = "";
  activeMqttUser = "";
  activeMqttPass = "";
  mqttConfigured = terminalClientEnabled();
  registered = false;
  lastMqttReconnectAt = 0;
  lastMqttErrorLogAt = 0;
  lastMqttState = 0;
  if (mqttConfigured) {
    logCaptureLn(String("MQTT 配置已更新: ") + mqttBrokerURL());
  } else {
    logCaptureLn(String("MQTT 配置已清空"));
  }
}

void terminalClientInit() {
  smsQueueInit();
  if (!terminalClientEnabled()) {
    logCaptureLn(String("中心终端客户端未启用：未配置 MQTT Broker"));
    return;
  }
  mqttConfigured = true;
  lastEsimProfileSyncAt = millis();
  lastIdentityRefreshAt = millis();
  logCaptureLn(String("中心终端 MQTT 客户端启用: ") + mqttBrokerURL());
  ensureMqttConnected();
}

void terminalClientService() {
  if (!terminalClientEnabled() || WiFi.status() != WL_CONNECTED) return;
  bool mqttReady = ensureMqttConnected();
  if (mqttReady) mqttClient.loop();
  if (mqttReady) flushSMSQueue();
  if (mqttReady) flushCommandResult();
  smsQueueService();
  unsigned long now = millis();
  if (mqttReady && now - lastHeartbeatAt > HEARTBEAT_INTERVAL_MS) terminalHeartbeat(false);
}

void terminalClientLoop() {
  if (!terminalClientEnabled() || WiFi.status() != WL_CONNECTED) return;
  terminalClientService();
  executePendingCommand();
  if (esimProfileSyncPending && mqttClient.connected() && !commandExecuting && !apduSessionConnected &&
      millis() - lastEsimProfileSyncAt >= ESIM_PROFILE_SYNC_RETRY_MS) {
    terminalSyncEsimProfiles();
  }
  unsigned long now = millis();
  if (!registered || (lastRegisterAt > 0 && now - lastRegisterAt > REGISTER_INTERVAL_MS)) terminalRegister();
  if (now - lastEsimProfileSyncAt > ESIM_PROFILE_SYNC_INTERVAL_MS) terminalSyncEsimProfiles();
  unsigned long identityInterval = identitySettleUntil > 0 && (long)(identitySettleUntil - now) > 0
                                     ? IDENTITY_SETTLE_INTERVAL_MS
                                     : IDENTITY_REFRESH_INTERVAL_MS;
  if (now - lastIdentityRefreshAt > identityInterval) {
    refreshIdentityCache();
    terminalHeartbeat(false);
  }
  if (now - lastLogFlushAt > LOG_FLUSH_INTERVAL_MS) flushLogs();
}
