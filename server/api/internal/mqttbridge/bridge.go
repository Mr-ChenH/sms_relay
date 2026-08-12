package mqttbridge

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/store"
)

type Bridge struct {
	store    *store.Store
	client   mqtt.Client
	broker   string
	clientID string
	username string
	password string
}

func New(store *store.Store, broker, clientID, username, password string) *Bridge {
	return &Bridge{store: store, broker: strings.TrimSpace(broker), clientID: firstNonEmpty(clientID, "sms-hub-api"), username: username, password: password}
}

func (b *Bridge) Enabled() bool {
	return b.broker != ""
}

func (b *Bridge) Start(ctx context.Context) {
	if !b.Enabled() {
		return
	}
	opts := mqtt.NewClientOptions().AddBroker(b.broker).SetClientID(b.clientID)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(3 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(20 * time.Second)
	if b.username != "" {
		opts.SetUsername(b.username)
		opts.SetPassword(b.password)
	}
	opts.OnConnect = func(client mqtt.Client) {
		log.Printf("mqtt connected broker=%s", b.broker)
		b.subscribe(client)
	}
	opts.OnConnectionLost = func(_ mqtt.Client, err error) {
		log.Printf("mqtt connection lost: %v", err)
	}

	b.client = mqtt.NewClient(opts)
	go func() {
		for {
			if token := b.client.Connect(); token.WaitTimeout(6*time.Second) && token.Error() != nil {
				log.Printf("mqtt bridge connect failed broker=%s: %v", b.broker, token.Error())
			}
			if b.client.IsConnectionOpen() {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
		<-ctx.Done()
		b.client.Disconnect(250)
	}()
}

func (b *Bridge) PublishCommand(command model.DeviceCommand, device model.Device) {
	if b.client == nil || !b.client.IsConnectionOpen() {
		return
	}
	payload, err := json.Marshal(command)
	if err != nil {
		log.Printf("mqtt marshal command failed id=%s: %v", command.ID, err)
		return
	}
	topic := "sms-hub/devices/" + device.DeviceID + "/commands"
	token := b.client.Publish(topic, 1, false, payload)
	token.WaitTimeout(2 * time.Second)
	if token.Error() != nil {
		log.Printf("mqtt publish command failed id=%s topic=%s: %v", command.ID, topic, token.Error())
	}
}

func (b *Bridge) subscribe(client mqtt.Client) {
	subs := map[string]byte{
		"sms-hub/devices/+/register":          1,
		"sms-hub/devices/+/heartbeat":         1,
		"sms-hub/devices/+/sms":               1,
		"sms-hub/devices/+/logs":              1,
		"sms-hub/devices/+/esim/profiles":     1,
		"sms-hub/devices/+/commands/+/result": 1,
		"sms-hub/devices/+/status":            1,
	}
	if token := client.SubscribeMultiple(subs, b.handleMessage); token.Wait() && token.Error() != nil {
		log.Printf("mqtt subscribe failed: %v", token.Error())
	}
}

func (b *Bridge) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) < 4 || parts[0] != "sms-hub" || parts[1] != "devices" {
		return
	}
	deviceID := parts[2]
	action := parts[3]
	payload := msg.Payload()

	switch action {
	case "register":
		var req model.TerminalRegisterRequest
		if decode(payload, &req) {
			req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
			if _, err := b.store.RegisterTerminal(req); err != nil {
				log.Printf("mqtt register failed device=%s: %v", deviceID, err)
			}
		}
	case "heartbeat":
		var req model.TerminalHeartbeatRequest
		if decode(payload, &req) {
			req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
			if _, err := b.store.Heartbeat(req); err != nil {
				log.Printf("mqtt heartbeat failed device=%s: %v", deviceID, err)
			}
		}
	case "sms":
		var req model.TerminalSMSRequest
		if decode(payload, &req) {
			req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
			if _, _, err := b.store.StoreTerminalSMS(req); err != nil {
				log.Printf("mqtt sms failed device=%s: %v", deviceID, err)
			}
		}
	case "logs":
		var req model.TerminalLogRequest
		if decode(payload, &req) {
			req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
			if err := b.store.StoreTerminalLogs(req); err != nil {
				log.Printf("mqtt logs failed device=%s: %v", deviceID, err)
			}
		}
	case "esim":
		if len(parts) >= 5 && parts[4] == "profiles" {
			var req model.TerminalEsimProfilesRequest
			if decode(payload, &req) {
				req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
				if _, err := b.store.ReplaceTerminalEsimProfiles(req); err != nil {
					log.Printf("mqtt esim profiles failed device=%s: %v", deviceID, err)
				}
			}
		}
	case "commands":
		if len(parts) >= 6 && parts[5] == "result" {
			var req model.TerminalCommandResultRequest
			if decode(payload, &req) {
				req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
				if _, err := b.store.CompleteCommand(parts[4], req); err != nil {
					log.Printf("mqtt command result failed device=%s command=%s: %v", deviceID, parts[4], err)
				}
			}
		}
	case "status":
		status := strings.TrimSpace(string(payload))
		switch status {
		case "offline":
			if err := b.store.MarkTerminalOffline(deviceID); err != nil {
				log.Printf("mqtt offline failed device=%s: %v", deviceID, err)
			}
		case "online":
			if err := b.store.MarkTerminalOnline(deviceID); err != nil {
				log.Printf("mqtt online failed device=%s: %v", deviceID, err)
			}
		}
	}
}

func decode(payload []byte, out interface{}) bool {
	if err := json.Unmarshal(payload, out); err != nil {
		log.Printf("mqtt decode failed: %v", err)
		return false
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
