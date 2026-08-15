package mqttbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"sms-forwarding/server/api/internal/lpa"
	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/store"
)

type Bridge struct {
	store       *store.Store
	client      mqtt.Client
	broker      string
	clientID    string
	username    string
	password    string
	apduSeq     atomic.Uint64
	apduMu      sync.Mutex
	apduPending map[string]chan lpa.APDUResponse
	deviceLocks sync.Map
}

func New(store *store.Store, broker, clientID, username, password string) *Bridge {
	return &Bridge{
		store: store, broker: strings.TrimSpace(broker), clientID: firstNonEmpty(clientID, "sms-hub-api"),
		username: username, password: password, apduPending: make(map[string]chan lpa.APDUResponse),
	}
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

func (b *Bridge) Exchange(ctx context.Context, deviceID string, request lpa.APDURequest) (lpa.APDUResponse, error) {
	if b.client == nil || !b.client.IsConnectionOpen() {
		return lpa.APDUResponse{}, errors.New("MQTT bridge is not connected")
	}
	lockValue, _ := b.deviceLocks.LoadOrStore(deviceID, &sync.Mutex{})
	deviceLock := lockValue.(*sync.Mutex)
	deviceLock.Lock()
	defer deviceLock.Unlock()

	requestID := fmt.Sprintf("apdu-%d", b.apduSeq.Add(1))
	responseCh := make(chan lpa.APDUResponse, 1)
	b.apduMu.Lock()
	b.apduPending[requestID] = responseCh
	b.apduMu.Unlock()
	defer func() {
		b.apduMu.Lock()
		delete(b.apduPending, requestID)
		b.apduMu.Unlock()
	}()

	payload, err := json.Marshal(map[string]interface{}{
		"requestId": requestID,
		"func":      request.Func,
		"param":     request.Param,
	})
	if err != nil {
		return lpa.APDUResponse{}, err
	}
	topic := "sms-hub/devices/" + deviceID + "/esim/apdu/request"
	token := b.client.Publish(topic, 1, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return lpa.APDUResponse{}, errors.New("timed out publishing APDU request")
	}
	if token.Error() != nil {
		return lpa.APDUResponse{}, token.Error()
	}

	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case response := <-responseCh:
		if response.ECode < 0 {
			return response, errors.New(firstNonEmpty(response.Error, "terminal APDU operation failed"))
		}
		return response, nil
	case <-ctx.Done():
		return lpa.APDUResponse{}, ctx.Err()
	case <-timer.C:
		return lpa.APDUResponse{}, errors.New("terminal APDU response timed out")
	}
}

func (b *Bridge) deliverAPDUResponse(payload []byte) {
	var response struct {
		RequestID string `json:"requestId"`
		ECode     int    `json:"ecode"`
		Data      string `json:"data"`
		Error     string `json:"error"`
	}
	if !decode(payload, &response) || response.RequestID == "" {
		return
	}
	b.apduMu.Lock()
	responseCh := b.apduPending[response.RequestID]
	b.apduMu.Unlock()
	if responseCh != nil {
		if response.ECode < 0 || response.Error != "" {
			log.Printf("mqtt APDU response failed request=%s ecode=%d error=%s", response.RequestID, response.ECode, response.Error)
		}
		select {
		case responseCh <- lpa.APDUResponse{ECode: response.ECode, Data: response.Data, Error: response.Error}:
		default:
		}
	}
}

func (b *Bridge) subscribe(client mqtt.Client) {
	subs := map[string]byte{
		"sms-hub/devices/+/register":           1,
		"sms-hub/devices/+/heartbeat":          1,
		"sms-hub/devices/+/sms":                1,
		"sms-hub/devices/+/logs":               1,
		"sms-hub/devices/+/esim/profiles":      1,
		"sms-hub/devices/+/esim/apdu/response": 1,
		"sms-hub/devices/+/commands/+/status":  1,
		"sms-hub/devices/+/commands/+/result":  1,
		"sms-hub/devices/+/status":             1,
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
		if len(parts) >= 6 && parts[4] == "apdu" && parts[5] == "response" {
			b.deliverAPDUResponse(payload)
		} else if len(parts) >= 5 && parts[4] == "profiles" {
			var req model.TerminalEsimProfilesRequest
			if decode(payload, &req) {
				req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
				if _, err := b.store.ReplaceTerminalEsimProfiles(req); err != nil {
					log.Printf("mqtt esim profiles failed device=%s: %v", deviceID, err)
				}
			}
		}
	case "commands":
		if len(parts) >= 6 && parts[5] == "status" {
			var req model.TerminalCommandResultRequest
			if decode(payload, &req) {
				req.DeviceID = firstNonEmpty(req.DeviceID, deviceID)
				if _, err := b.store.UpdateCommandStatus(parts[4], req); err != nil {
					log.Printf("mqtt command status failed device=%s command=%s: %v", deviceID, parts[4], err)
				}
			}
		} else if len(parts) >= 6 && parts[5] == "result" {
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
			// MQTT last-will can fire during a brief Wi-Fi reconnect. Let the
			// heartbeat timeout decide offline state to avoid UI flapping.
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
