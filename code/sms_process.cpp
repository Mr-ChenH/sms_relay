#include "sms_process.h"
#include "logger.h"
#include "terminal_client.h"

// 初始化长短信缓存
void initConcatBuffer() {
  for (int i = 0; i < MAX_CONCAT_MESSAGES; i++) {
    concatBuffer[i].inUse = false;
    concatBuffer[i].receivedParts = 0;
    for (int j = 0; j < MAX_CONCAT_PARTS; j++) {
      concatBuffer[i].parts[j].valid = false;
      concatBuffer[i].parts[j].text = "";
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
  }
}

// 检查长短信超时并转发
void checkConcatTimeout() {
  unsigned long now = millis();
  for (int i = 0; i < MAX_CONCAT_MESSAGES; i++) {
    if (concatBuffer[i].inUse) {
      if (now - concatBuffer[i].firstPartTime >= CONCAT_TIMEOUT_MS) {
        logCaptureLn(String("⏰ 长短信超时，强制转发不完整消息"));
        logCaptureF("  参考号: %d, 已收到: %d/%d\n", 
                      concatBuffer[i].refNumber,
                      concatBuffer[i].receivedParts,
                      concatBuffer[i].totalParts);
        
        // 合并已收到的分段
        String fullText = assembleConcatSms(i);
        
        // 处理短信内容
        processSmsContent(concatBuffer[i].sender.c_str(), 
                         fullText.c_str(), 
                         concatBuffer[i].timestamp.c_str());
        
        // 清空槽位
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
void processSmsContent(const char* sender, const char* text, const char* timestamp) {
  logCaptureLn(String("=== 处理短信内容 ==="));
  logCaptureLn(String("发送者: ") + String(sender));
  logCaptureLn(String("时间戳: ") + String(timestamp));
  logCaptureLn(String("内容: ") + String(text));
  logCaptureLn(String("===================="));
  terminalReportSMS(sender, text, timestamp);
}

static void handlePduLine(const String& line) {
  logCaptureLn(String("收到PDU数据: " + line));
  logCaptureLn(String("PDU长度: " + String(line.length()) + " 字符"));

  if (!pdu.decodePDU(line.c_str())) {
    logCaptureLn(String("❌ PDU解析失败！"));
    return;
  }

  logCaptureLn(String("✓ PDU解析成功"));
  logCaptureLn(String("=== 短信内容 ==="));
  logCaptureLn(String("发送者: " + String(pdu.getSender())));
  logCaptureLn(String("时间戳: " + String(pdu.getTimeStamp())));
  logCaptureLn(String("内容: " + String(pdu.getText())));

  int* concatInfo = pdu.getConcatInfo();
  int refNumber = concatInfo[0];
  int partNumber = concatInfo[1];
  int totalParts = concatInfo[2];

  logCaptureF("长短信信息: 参考号=%d, 当前=%d, 总计=%d\n", refNumber, partNumber, totalParts);
  logCaptureLn(String("==============="));

  if (totalParts > 1 && partNumber > 0) {
    if (totalParts > MAX_CONCAT_PARTS || partNumber > totalParts) {
      logCaptureF("长短信分段超出容量，按单段上报: %d/%d\n", partNumber, totalParts);
      processSmsContent(pdu.getSender(), pdu.getText(), pdu.getTimeStamp());
      return;
    }
    logCaptureF("收到长短信分段 %d/%d\n", partNumber, totalParts);

    int slot = findOrCreateConcatSlot(refNumber, pdu.getSender(), totalParts);
    if (slot < 0) {
      processSmsContent(pdu.getSender(), pdu.getText(), pdu.getTimeStamp());
      return;
    }
    int partIndex = partNumber - 1;
    if (partIndex >= 0 && partIndex < MAX_CONCAT_PARTS) {
      if (!concatBuffer[slot].parts[partIndex].valid) {
        concatBuffer[slot].parts[partIndex].valid = true;
        concatBuffer[slot].parts[partIndex].text = String(pdu.getText());
        concatBuffer[slot].receivedParts++;

        if (concatBuffer[slot].receivedParts == 1) {
          concatBuffer[slot].timestamp = String(pdu.getTimeStamp());
        }

        logCaptureF("  已缓存分段 %d，当前已收到 %d/%d\n",
                    partNumber,
                    concatBuffer[slot].receivedParts,
                    totalParts);
      } else {
        logCaptureF("  ⚠️ 分段 %d 已存在，跳过\n", partNumber);
      }
    }

    if (concatBuffer[slot].receivedParts >= totalParts) {
      logCaptureLn(String("✅ 长短信已收齐，开始合并转发"));

      String fullText = assembleConcatSms(slot);
      processSmsContent(concatBuffer[slot].sender.c_str(),
                        fullText.c_str(),
                        concatBuffer[slot].timestamp.c_str());

      clearConcatSlot(slot);
    }
  } else {
    processSmsContent(pdu.getSender(), pdu.getText(), pdu.getTimeStamp());
  }
}

// 处理URC和PDU
void checkSerial1URC() {
  static enum { IDLE,
                WAIT_PDU } state = IDLE;

  String line = readSerialLine(Serial1);
  if (line.length() == 0) return;

  // 打印到调试串口
  logCaptureLn(String("Debug> " + line));

  if (state == IDLE) {
    // 检测到短信上报URC头
    if (line.startsWith("+CMT:")) {
      logCaptureLn(String("检测到+CMT，等待PDU数据..."));
      state = WAIT_PDU;
    } else if (isHexString(line) && line.length() >= 20) {
      logCaptureLn(String("检测到无+CMT头的PDU行，按短信分段处理"));
      handlePduLine(line);
    }
  } else if (state == WAIT_PDU) {
    // 如果等待PDU时又来了新的+CMT头，继续等待下一行PDU
    if (line.startsWith("+CMT:")) {
      logCaptureLn(String("等待PDU时再次收到+CMT，继续等待PDU数据..."));
      return;
    }

    if (isHexString(line)) {
      handlePduLine(line);
      state = IDLE;
    } else {
      logCaptureLn(String("收到非PDU数据，返回IDLE状态"));
      state = IDLE;
    }
  }
}
