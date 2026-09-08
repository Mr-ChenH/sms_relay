#include "sms_process.h"
#include "logger.h"
#include "modem.h"
#include "terminal_client.h"

// 初始化长短信缓存
void initConcatBuffer() {
  for (int i = 0; i < MAX_CONCAT_MESSAGES; i++) {
    concatBuffer[i].inUse = false;
    concatBuffer[i].receivedParts = 0;
    for (int j = 0; j < MAX_CONCAT_PARTS; j++) {
      concatBuffer[i].parts[j].valid = false;
      concatBuffer[i].parts[j].text = "";
      concatBuffer[i].storageIndexes[j] = 0;
    }
  }
}

// 查找或创建长短信缓存槽位
int findOrCreateConcatSlot(int refNumber, const char* sender, int totalParts) {
  if (totalParts < 1 || totalParts > MAX_CONCAT_PARTS) return -1;

  // 先查找是否已存在
  for (int i = 0; i < MAX_CONCAT_MESSAGES; i++) {
    if (concatBuffer[i].inUse && 
        concatBuffer[i].refNumber == refNumber &&
        concatBuffer[i].totalParts == totalParts &&
        concatBuffer[i].sender.equals(sender)) {
      return i;
    }
  }
  
  // 查找空闲槽位
  for (int i = 0; i < MAX_CONCAT_MESSAGES; i++) {
    if (!concatBuffer[i].inUse) {
      concatBuffer[i].inUse = true;
      concatBuffer[i].refNumber = refNumber;
      concatBuffer[i].sender = String(sender);
      concatBuffer[i].totalParts = totalParts;
      concatBuffer[i].receivedParts = 0;
      concatBuffer[i].firstPartTime = millis();
      for (int j = 0; j < MAX_CONCAT_PARTS; j++) {
        concatBuffer[i].parts[j].valid = false;
        concatBuffer[i].parts[j].text = "";
        concatBuffer[i].storageIndexes[j] = 0;
      }
      return i;
    }
  }
  
  // 没有空闲槽位，查找最老的槽位覆盖
  int oldestSlot = 0;
  unsigned long oldestTime = concatBuffer[0].firstPartTime;
  for (int i = 1; i < MAX_CONCAT_MESSAGES; i++) {
    if (concatBuffer[i].firstPartTime < oldestTime) {
      oldestTime = concatBuffer[i].firstPartTime;
      oldestSlot = i;
    }
  }
  
  // 覆盖最老的槽位
  logCaptureLn(String("⚠️ 长短信缓存已满，覆盖最老的槽位"));
  concatBuffer[oldestSlot].inUse = true;
  concatBuffer[oldestSlot].refNumber = refNumber;
  concatBuffer[oldestSlot].sender = String(sender);
  concatBuffer[oldestSlot].totalParts = totalParts;
  concatBuffer[oldestSlot].receivedParts = 0;
  concatBuffer[oldestSlot].firstPartTime = millis();
  for (int j = 0; j < MAX_CONCAT_PARTS; j++) {
    concatBuffer[oldestSlot].parts[j].valid = false;
    concatBuffer[oldestSlot].parts[j].text = "";
    concatBuffer[oldestSlot].storageIndexes[j] = 0;
  }
  return oldestSlot;
}

// 合并长短信各分段
String assembleConcatSms(int slot) {
  if (slot < 0 || slot >= MAX_CONCAT_MESSAGES) return "";
  int totalParts = concatBuffer[slot].totalParts;
  if (totalParts < 0) totalParts = 0;
  if (totalParts > MAX_CONCAT_PARTS) totalParts = MAX_CONCAT_PARTS;
  String result = "";
  for (int i = 0; i < totalParts; i++) {
    if (concatBuffer[slot].parts[i].valid) {
      result += concatBuffer[slot].parts[i].text;
    } else {
      result += "[缺失分段" + String(i + 1) + "]";
    }
  }
  return result;
}

// 清空长短信槽位
void clearConcatSlot(int slot) {
  if (slot < 0 || slot >= MAX_CONCAT_MESSAGES) return;
  concatBuffer[slot].inUse = false;
  concatBuffer[slot].receivedParts = 0;
  concatBuffer[slot].sender = "";
  concatBuffer[slot].timestamp = "";
  for (int j = 0; j < MAX_CONCAT_PARTS; j++) {
    concatBuffer[slot].parts[j].valid = false;
    concatBuffer[slot].parts[j].text = "";
    concatBuffer[slot].storageIndexes[j] = 0;
  }
}

// 检查长短信超时并转发
void checkConcatTimeout() {
  unsigned long now = millis();
  for (int i = 0; i < MAX_CONCAT_MESSAGES; i++) {
    if (concatBuffer[i].inUse) {
      if (now - concatBuffer[i].firstPartTime >= CONCAT_TIMEOUT_MS) {
        logCaptureLn(String("长短信等待超时，保留模组分段并等待后续补扫"));
        logCaptureF("  参考号: %d, 已收到: %d/%d\n",
                    concatBuffer[i].refNumber,
                    concatBuffer[i].receivedParts,
                    concatBuffer[i].totalParts);
        // 未收齐时绝不上传占位消息，也不删除模组记录。后续人工诊断
        // 仍可从ME读取原始分段，避免把不完整内容当作正常短信上报。
        clearConcatSlot(i);
      }
    }
  }
}

// 读取串口一行（含回车换行），返回行字符串，无新行时返回空
String readSerialLine(HardwareSerial& port) {
  static char lineBuf[SERIAL_BUFFER_SIZE];
  static int linePos = 0;

  while (port.available()) {
    char c = port.read();
    if (c == '\n') {
      lineBuf[linePos] = 0;
      String res = String(lineBuf);
      linePos = 0;
      return res;
    } else if (c != '\r') {  // 跳过\r
      if (linePos < SERIAL_BUFFER_SIZE - 1)
        lineBuf[linePos++] = c;
      else
        linePos = 0;  //超长报错保护，重头计
    }
  }
  return "";
}

// 检查字符串是否为有效的十六进制PDU数据
bool isHexString(const String& str) {
  if (str.length() == 0) return false;
  for (unsigned int i = 0; i < str.length(); i++) {
    char c = str.charAt(i);
    if (!((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f'))) {
      return false;
    }
  }
  return true;
}

// 处理最终短信内容。采集端不做过滤或远程命令解释，统一交给中心服务审计和路由。
bool processSmsContent(const char* sender, const char* text, const char* timestamp) {
  logCaptureLn(String("=== 处理短信内容 ==="));
  logCaptureLn(String("发送者: ") + String(sender));
  logCaptureLn(String("时间戳: ") + String(timestamp));
  logCaptureLn(String("内容: ") + String(text));
  logCaptureLn(String("===================="));
  return terminalReportSMS(sender, text, timestamp);
}

static bool deleteStoredSMS(int index) {
  if (index <= 0) return true;
  String response = sendATCommand((String("AT+CMGD=") + index).c_str(), 5000);
  if (response.indexOf("OK") >= 0) return true;
  logCaptureLn(String("短信存储删除失败 index=") + index + ": " + response);
  return false;
}

static SmsPduResult handlePduLine(const String& line, int storageIndex = 0) {
  logCaptureLn(String("收到PDU数据: ") + line);
  logCaptureLn(String("PDU长度: ") + line.length() + " 字符");

  if (!pdu.decodePDU(line.c_str())) {
    logCaptureLn(String("PDU解析失败"));
    return SMS_PDU_FAILED;
  }

  logCaptureLn(String("PDU解析成功"));
  logCaptureLn(String("=== 短信内容 ==="));
  logCaptureLn(String("发送者: ") + pdu.getSender());
  logCaptureLn(String("时间戳: ") + pdu.getTimeStamp());
  logCaptureLn(String("内容: ") + pdu.getText());

  int* concatInfo = pdu.getConcatInfo();
  int refNumber = concatInfo[0];
  int partNumber = concatInfo[1];
  int totalParts = concatInfo[2];
  logCaptureF("长短信信息: 参考号=%d, 当前=%d, 总计=%d\n", refNumber, partNumber, totalParts);

  if (totalParts <= 1 || partNumber <= 0) {
    return processSmsContent(pdu.getSender(), pdu.getText(), pdu.getTimeStamp())
             ? SMS_PDU_QUEUED : SMS_PDU_FAILED;
  }
  if (totalParts > MAX_CONCAT_PARTS || partNumber > totalParts) {
    logCaptureF("长短信分段超出容量，按单段上报: %d/%d\n", partNumber, totalParts);
    return processSmsContent(pdu.getSender(), pdu.getText(), pdu.getTimeStamp())
             ? SMS_PDU_QUEUED : SMS_PDU_FAILED;
  }

  int slot = findOrCreateConcatSlot(refNumber, pdu.getSender(), totalParts);
  if (slot < 0) return SMS_PDU_FAILED;
  int partIndex = partNumber - 1;
  if (!concatBuffer[slot].parts[partIndex].valid) {
    concatBuffer[slot].parts[partIndex].valid = true;
    concatBuffer[slot].parts[partIndex].text = String(pdu.getText());
    concatBuffer[slot].receivedParts++;
    if (concatBuffer[slot].receivedParts == 1) {
      concatBuffer[slot].timestamp = String(pdu.getTimeStamp());
    }
  }
  if (storageIndex > 0) concatBuffer[slot].storageIndexes[partIndex] = storageIndex;
  logCaptureF("已缓存分段 %d，当前已收到 %d/%d\n",
              partNumber, concatBuffer[slot].receivedParts, totalParts);

  if (concatBuffer[slot].receivedParts < totalParts) return SMS_PDU_BUFFERED;

  String fullText = assembleConcatSms(slot);
  bool queued = processSmsContent(concatBuffer[slot].sender.c_str(),
                                  fullText.c_str(),
                                  concatBuffer[slot].timestamp.c_str());
  if (!queued) return SMS_PDU_FAILED;
  for (int i = 0; i < totalParts; i++) deleteStoredSMS(concatBuffer[slot].storageIndexes[i]);
  clearConcatSlot(slot);
  return SMS_PDU_QUEUED;
}

// URC 解析状态机（文件级状态，供 checkSerial1URC 与 drainSerial1Urx 共用）
static enum { URC_IDLE,
              URC_WAIT_PDU } urcState = URC_IDLE;
static unsigned long lastStorageScanAt = 0;
static bool storageScanRequested = true;
static unsigned long storageScanRequestedAt = 0;
static const unsigned long STORAGE_SCAN_DEBOUNCE_MS = 5000;
static const unsigned long STORAGE_SCAN_INTERVAL_MS = 60000;
static const int MAX_STORAGE_MESSAGES_PER_SCAN = 8;

// 处理一行串口数据（+CMT 头 / PDU 数据行 / 无头 PDU 行）
static void processSerial1Line(const String& line) {
  if (line.length() == 0) return;

  // 打印到调试串口
  logCaptureLn(String("Debug> " + line));

  if (urcState == URC_IDLE) {
    if (line.startsWith("+CMTI:")) {
      logCaptureLn(String("检测到+CMTI，安排扫描模组短信存储"));
      storageScanRequested = true;
      storageScanRequestedAt = millis();
      storageScanRequestedAt = millis();
    } else if (line.startsWith("+CMT:")) {
      logCaptureLn(String("检测到+CMT，等待PDU数据..."));
      urcState = URC_WAIT_PDU;
    } else if (isHexString(line) && line.length() >= 20) {
      logCaptureLn(String("检测到无+CMT头的PDU行，按短信分段处理"));
      handlePduLine(line);
    }
  } else if (urcState == URC_WAIT_PDU) {
    // 如果等待PDU时又来了新的+CMT头，继续等待下一行PDU
    if (line.startsWith("+CMT:")) {
      logCaptureLn(String("等待PDU时再次收到+CMT，继续等待PDU数据..."));
      return;
    }

    if (isHexString(line)) {
      handlePduLine(line);
      urcState = URC_IDLE;
    } else {
      logCaptureLn(String("收到非PDU数据，返回IDLE状态"));
      urcState = URC_IDLE;
    }
  }
}

void smsStorageService() {
  unsigned long now = millis();
  if (!modemReady) return;
  if (storageScanRequested && storageScanRequestedAt > 0 && now - storageScanRequestedAt < STORAGE_SCAN_DEBOUNCE_MS) return;
  bool periodicDue = lastStorageScanAt == 0 || now - lastStorageScanAt >= STORAGE_SCAN_INTERVAL_MS;
  bool requestedDue = storageScanRequested &&
                      (storageScanRequestedAt == 0 || now - storageScanRequestedAt >= STORAGE_SCAN_DEBOUNCE_MS);
  if (!periodicDue && !requestedDue) return;
  storageScanRequested = false;
  storageScanRequestedAt = 0;
  storageScanRequestedAt = 0;
  lastStorageScanAt = now;

  String response = sendATCommand("AT+CMGL=0", 15000);
  int indexes[MAX_STORAGE_MESSAGES_PER_SCAN];
  int processed = 0;
  int pendingIndex = -1;
  int start = 0;
  while (start <= response.length() && processed < MAX_STORAGE_MESSAGES_PER_SCAN) {
    int end = response.indexOf('\n', start);
    if (end < 0) end = response.length();
    String line = response.substring(start, end);
    line.replace("\r", "");
    line.trim();
    if (line.startsWith("+CMGL:")) {
      int colon = line.indexOf(':');
      int comma = line.indexOf(',', colon + 1);
      pendingIndex = comma > colon ? line.substring(colon + 1, comma).toInt() : -1;
    } else if (pendingIndex > 0 && isHexString(line) && line.length() >= 20) {
      SmsPduResult result = handlePduLine(line, pendingIndex);
      if (result == SMS_PDU_QUEUED) {
        int* concatInfo = pdu.getConcatInfo();
        if (concatInfo[2] <= 1) indexes[processed++] = pendingIndex;
      }
      pendingIndex = -1;
    }
    if (end == response.length()) break;
    start = end + 1;
  }

  for (int i = 0; i < processed; i++) {
    deleteStoredSMS(indexes[i]);
  }
  if (processed > 0) terminalReportLog("info", String("stored SMS recovered count=") + processed);
}

// 处理URC和PDU
void checkSerial1URC() {
  processSerial1Line(readSerialLine(Serial1));
}

// 在 AT 命令清空 Serial1 之前调用：先把缓冲区中完整的 URC（+CMT: 头 + PDU 数据行）
// 收完处理掉，避免清空操作把待收短信丢掉。无待处理数据时立即返回，不阻塞 AT 流程。
void drainSerial1Urx() {
  unsigned long start = millis();
  unsigned long lastDataAt = millis();
  while (millis() - start < 2000) {
    String line = readSerialLine(Serial1);
    if (line.length() > 0) {
      lastDataAt = millis();
      processSerial1Line(line);
      continue;
    }
    if (Serial1.available() == 0) {
      if (urcState == URC_WAIT_PDU) {
        // 已读到 +CMT: 头，PDU 数据行即将到达，再等一会
        delay(1);
        continue;
      }
      return;
    }
    // 有未成行的部分数据：数据仍在持续到达时继续等它收完，长时间无进展则放弃
    if (millis() - lastDataAt >= 100) return;
    delay(1);
  }
}
