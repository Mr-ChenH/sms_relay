#include "sms_queue.h"
#include "logger.h"

#include <ArduinoJson.h>
#include <LittleFS.h>

static const char* QUEUE_PATH = "/sms-queue.json";
static const char* QUEUE_TEMP_PATH = "/sms-queue.tmp";
static const char* QUEUE_BACKUP_PATH = "/sms-queue.bak";
static const size_t MAX_QUEUE_ITEMS = 32;
static const unsigned long FLUSH_DELAY_MS = 5000;
static const size_t FORCE_FLUSH_CHANGES = 8;

static QueuedSMS queueItems[MAX_QUEUE_ITEMS];
static size_t queueCount = 0;
static bool storageReady = false;
static bool dirty = false;
static unsigned long dirtySince = 0;
static size_t pendingChanges = 0;

static void markDirty() {
  if (!dirty) dirtySince = millis();
  dirty = true;
  pendingChanges++;
}

static void clearItem(QueuedSMS& item) {
  item.messageId = "";
  item.sender = "";
  item.body = "";
  item.timestamp = "";
}

static bool persistQueue() {
  if (!storageReady || !dirty) return storageReady;

  if (queueCount == 0) {
    LittleFS.remove(QUEUE_TEMP_PATH);
    LittleFS.remove(QUEUE_PATH);
    dirty = false;
    pendingChanges = 0;
    return true;
  }

  File file = LittleFS.open(QUEUE_TEMP_PATH, FILE_WRITE);
  if (!file) {
    logCaptureLn("无法创建短信队列临时文件");
    return false;
  }

  JsonDocument doc;
  doc["version"] = 1;
  JsonArray items = doc["items"].to<JsonArray>();
  for (size_t i = 0; i < queueCount; i++) {
    JsonObject row = items.add<JsonObject>();
    row["id"] = queueItems[i].messageId;
    row["sender"] = queueItems[i].sender;
    row["body"] = queueItems[i].body;
    row["timestamp"] = queueItems[i].timestamp;
  }

  bool ok = serializeJson(doc, file) > 0;
  file.flush();
  file.close();
  if (!ok) {
    LittleFS.remove(QUEUE_TEMP_PATH);
    logCaptureLn("短信队列序列化失败");
    return false;
  }

  LittleFS.remove(QUEUE_BACKUP_PATH);
  if (LittleFS.exists(QUEUE_PATH) && !LittleFS.rename(QUEUE_PATH, QUEUE_BACKUP_PATH)) {
    LittleFS.remove(QUEUE_TEMP_PATH);
    logCaptureLn("短信队列备份失败");
    return false;
  }
  if (!LittleFS.rename(QUEUE_TEMP_PATH, QUEUE_PATH)) {
    if (LittleFS.exists(QUEUE_BACKUP_PATH)) LittleFS.rename(QUEUE_BACKUP_PATH, QUEUE_PATH);
    logCaptureLn("短信队列文件替换失败");
    return false;
  }
  LittleFS.remove(QUEUE_BACKUP_PATH);

  dirty = false;
  pendingChanges = 0;
  return true;
}

bool smsQueueInit() {
  queueCount = 0;
  storageReady = LittleFS.begin(true);
  if (!storageReady) {
    logCaptureLn("LittleFS 初始化失败，短信队列仅使用 RAM");
    return false;
  }

  if (!LittleFS.exists(QUEUE_PATH) && LittleFS.exists(QUEUE_BACKUP_PATH)) {
    LittleFS.rename(QUEUE_BACKUP_PATH, QUEUE_PATH);
  }
  if (!LittleFS.exists(QUEUE_PATH)) return true;
  File file = LittleFS.open(QUEUE_PATH, FILE_READ);
  if (!file) return false;

  JsonDocument doc;
  DeserializationError error = deserializeJson(doc, file);
  file.close();
  if (error || (doc["version"] | 0) != 1) {
    logCaptureLn("短信队列文件损坏，已隔离");
    LittleFS.remove(QUEUE_TEMP_PATH);
    LittleFS.rename(QUEUE_PATH, QUEUE_TEMP_PATH);
    return false;
  }

  JsonArrayConst items = doc["items"].as<JsonArrayConst>();
  for (JsonObjectConst row : items) {
    if (queueCount >= MAX_QUEUE_ITEMS) break;
    const char* id = row["id"] | "";
    const char* sender = row["sender"] | "";
    const char* body = row["body"] | "";
    const char* timestamp = row["timestamp"] | "";
    if (!id[0] || strlen(id) > 96 || strlen(sender) > 48 || strlen(body) > 2048 || strlen(timestamp) > 48) continue;
    queueItems[queueCount++] = {String(id), String(sender), String(body), String(timestamp)};
  }

  if (queueCount > 0) logCaptureLn(String("已恢复短信待发队列: ") + String(queueCount));
  return true;
}

size_t smsQueueSize() {
  return queueCount;
}

const QueuedSMS* smsQueueFront() {
  return queueCount > 0 ? &queueItems[0] : nullptr;
}

bool smsQueuePush(const QueuedSMS& item) {
  if (queueCount >= MAX_QUEUE_ITEMS) return false;
  queueItems[queueCount++] = item;
  markDirty();
  return true;
}

bool smsQueuePop() {
  if (queueCount == 0) return false;
  clearItem(queueItems[0]);
  for (size_t i = 1; i < queueCount; i++) queueItems[i - 1] = queueItems[i];
  queueCount--;
  clearItem(queueItems[queueCount]);
  markDirty();
  return true;
}

void smsQueueService() {
  if (!dirty) return;
  if (pendingChanges >= FORCE_FLUSH_CHANGES || millis() - dirtySince >= FLUSH_DELAY_MS) persistQueue();
}

void smsQueueFlushNow() {
  persistQueue();
}
