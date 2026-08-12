#ifndef SMS_QUEUE_H
#define SMS_QUEUE_H

#include <Arduino.h>

struct QueuedSMS {
  String messageId;
  String sender;
  String body;
  String timestamp;
};

bool smsQueueInit();
size_t smsQueueSize();
const QueuedSMS* smsQueueFront();
bool smsQueuePush(const QueuedSMS& item);
bool smsQueuePop();
void smsQueueService();
void smsQueueFlushNow();

#endif
