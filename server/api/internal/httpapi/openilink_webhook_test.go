package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/notify"
	"sms-forwarding/server/api/internal/store"
)

func TestOpenILinkWebhookURLVerification(t *testing.T) {
	s, handler := newOpenILinkWebhookTestServer(t, []string{"get_overview"})
	_ = s
	request := httptest.NewRequest(http.MethodPost, "/api/integrations/openilink/webhook", bytes.NewBufferString(`{"v":1,"type":"url_verification","challenge":"abc123"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != `{"challenge":"abc123"}` {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestOpenILinkWebhookRejectsUnlistedInstallation(t *testing.T) {
	_, handler := newOpenILinkWebhookTestServer(t, []string{"get_overview"})
	body := openILinkCommandBody(t, "evt-1", "inst-denied", "get_overview", nil)
	request := signedOpenILinkRequest(t, body, "inst-denied", "webhook-secret", time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestOpenILinkWebhookRejectsExpiredSignature(t *testing.T) {
	_, handler := newOpenILinkWebhookTestServer(t, []string{"get_overview"})
	body := openILinkCommandBody(t, "evt-1", "inst-allowed", "get_overview", nil)
	request := signedOpenILinkRequest(t, body, "inst-allowed", "webhook-secret", time.Now().Add(-10*time.Minute))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestOpenILinkWebhookDeduplicatesSendSMS(t *testing.T) {
	s, handler := newOpenILinkWebhookTestServer(t, []string{"send_sms"})
	device, err := s.RegisterTerminal(model.TerminalRegisterRequest{DeviceID: "terminal-1", Name: "Desk"})
	if err != nil {
		t.Fatal(err)
	}
	body := openILinkCommandBody(t, "evt-send-1", "inst-allowed", "send_sms", map[string]interface{}{
		"deviceId": device.ID,
		"phone":    "+8613800000000",
		"body":     "hello",
	})

	for i := 0; i < 2; i++ {
		request := signedOpenILinkRequest(t, body, "inst-allowed", "webhook-secret", time.Now())
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, body = %s", i+1, response.Code, response.Body.String())
		}
		if i == 0 {
			// Simulate an API process restart, which clears the in-memory reply cache.
			handler = New(s, notify.NewClient("")).Handler()
		}
	}
	commands := s.Commands()
	if len(commands) != 1 || commands[0].Type != "send_sms" {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestOpenILinkWebhookHonorsCapabilitySelection(t *testing.T) {
	s, handler := newOpenILinkWebhookTestServer(t, []string{"get_overview"})
	device, err := s.RegisterTerminal(model.TerminalRegisterRequest{DeviceID: "terminal-1", Name: "Desk"})
	if err != nil {
		t.Fatal(err)
	}
	body := openILinkCommandBody(t, "evt-send-disabled", "inst-allowed", "send_sms", map[string]interface{}{
		"deviceId": device.ID, "phone": "+8613800000000", "body": "hello",
	})
	request := signedOpenILinkRequest(t, body, "inst-allowed", "webhook-secret", time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(s.Commands()) != 0 {
		t.Fatalf("status = %d, commands = %#v", response.Code, s.Commands())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("未启用功能")) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestOpenILinkWebhookHelpListsOnlyEnabledCommands(t *testing.T) {
	_, handler := newOpenILinkWebhookTestServer(t, []string{"get_overview", "list_devices"})
	body := openILinkCommandBody(t, "evt-help", "inst-allowed", "help", nil)
	request := signedOpenILinkRequest(t, body, "inst-allowed", "webhook-secret", time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"/help", "/get_overview", "/list_devices"} {
		if !strings.Contains(result["reply"], wanted) {
			t.Fatalf("help reply missing %q: %s", wanted, result["reply"])
		}
	}
	if strings.Contains(result["reply"], "/send_sms") {
		t.Fatalf("help reply exposes disabled command: %s", result["reply"])
	}
}

func TestOpenILinkWebhookHelpDescribesCommand(t *testing.T) {
	_, handler := newOpenILinkWebhookTestServer(t, []string{"send_sms"})
	body := openILinkCommandBody(t, "evt-help-send", "inst-allowed", "help", map[string]interface{}{"command": "send_sms"})
	request := signedOpenILinkRequest(t, body, "inst-allowed", "webhook-secret", time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result["reply"], "/send_sms <deviceId> <phone> <message>") || !strings.Contains(result["reply"], "运营商费用") {
		t.Fatalf("reply = %s", result["reply"])
	}
}

func TestFormatOpenILinkDevicesIsReadable(t *testing.T) {
	lastSeen := time.Date(2026, time.September, 27, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	result := formatOpenILinkDevices([]model.Device{{
		ID: "dev-101", Name: "Bedroom", Status: "online", PhoneNumber: "+8613800000000",
		Operator: "Carrier", CellularRSSI: -83, LastSeenAt: lastSeen,
	}})
	for _, wanted := range []string{"终端列表（1）", "1. Bedroom [在线]", "ID：dev-101", "号码：+8613800000000", "蜂窝信号：-83 dBm", "最后在线：2026-09-27 08:30"} {
		if !strings.Contains(result, wanted) {
			t.Fatalf("result missing %q:\n%s", wanted, result)
		}
	}
	if strings.ContainsAny(result, "{}") {
		t.Fatalf("result contains raw JSON: %s", result)
	}
}

func TestFormatOpenILinkSMSUsesBlocksAndTruncatesBody(t *testing.T) {
	result := formatOpenILinkSMS(model.SMSList{
		Items: []model.SMSMessage{{DeviceName: "Phone", Sender: "10086", Recipient: "+86138", Body: strings.Repeat("长", 200), Timestamp: time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)}},
		Total: 2, Page: 1, PageSize: 1,
	}, "验证码")
	for _, wanted := range []string{"短信搜索：验证码", "第 1 页 · 共 2 条", "1. 2026-09-27 09:00 · Phone", "来自：10086", "发往：+86138", "还有更多结果"} {
		if !strings.Contains(result, wanted) {
			t.Fatalf("result missing %q:\n%s", wanted, result)
		}
	}
	if !strings.Contains(result, "...") {
		t.Fatalf("long message was not truncated: %s", result)
	}
}

func TestOpenILinkToolsAlwaysIncludeHelp(t *testing.T) {
	tools := openILinkTools([]string{"list_devices"})
	if len(tools) != 2 || tools[0].Name != "help" || tools[1].Name != "list_devices" {
		t.Fatalf("tools = %#v", tools)
	}
}

func newOpenILinkWebhookTestServer(t *testing.T, capabilities []string) (*store.Store, http.Handler) {
	t.Helper()
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "smshub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	_, err = s.CreateAppriseService(model.CreateAppriseServiceRequest{
		Type: "openilink", Name: "OpeniLink", BaseURL: "http://openilink.test", Enabled: true,
		OpenILinkInboundEnabled: true, OpenILinkWebhookSecret: "webhook-secret",
		OpenILinkInstallationIDs: []string{"inst-allowed"}, OpenILinkCapabilities: capabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, New(s, notify.NewClient("")).Handler()
}

func openILinkCommandBody(t *testing.T, eventID, installationID, command string, args map[string]interface{}) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"v": 1, "type": "event", "trace_id": "tr-test", "installation_id": installationID,
		"bot": map[string]string{"id": "bot-1"},
		"event": map[string]interface{}{
			"type": "command", "id": eventID, "timestamp": time.Now().Unix(),
			"data": map[string]interface{}{"command": command, "text": "", "args": args, "sender": map[string]string{"id": "user-1", "role": "agent"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func signedOpenILinkRequest(t *testing.T, body []byte, installationID, secret string, requestTime time.Time) *http.Request {
	t.Helper()
	timestamp := strconv.FormatInt(requestTime.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%s:%s", timestamp, body)))
	request := httptest.NewRequest(http.MethodPost, "/api/integrations/openilink/webhook", bytes.NewReader(body))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Installation-Id", installationID)
	return request
}
