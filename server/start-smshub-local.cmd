@echo off
set SMS_HUB_MQTT_BROKER=tcp://127.0.0.1:1883
set SMS_HUB_PUBLIC_BASE_URL=http://192.168.2.11:8080
set SMS_HUB_PUBLIC_MQTT_BROKER=mqtt://192.168.2.11:1883
cd /d C:\Users\threeTeeth\workSpace\github\sms_forwarding\server
start "smshub" smshub.exe
