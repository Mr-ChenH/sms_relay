package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/notify"
)

const (
	openILinkWebhookMaxBody = 1 << 20
	openILinkTimestampSkew  = 5 * time.Minute
	openILinkReplyCacheTTL  = 24 * time.Hour
)

type openILinkCachedReply struct {
	status    int
	body      []byte
	createdAt time.Time
}

type openILinkEnvelope struct {
	V              int            `json:"v"`
	Type           string         `json:"type"`
	Challenge      string         `json:"challenge"`
	TraceID        string         `json:"trace_id"`
	InstallationID string         `json:"installation_id"`
	Event          openILinkEvent `json:"event"`
}

type openILinkEvent struct {
	Type string          `json:"type"`
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

type openILinkCommand struct {
	Command string                 `json:"command"`
	Text    string                 `json:"text"`
	Args    map[string]interface{} `json:"args"`
}

func (s *Server) syncOpenILinkTools(w http.ResponseWriter, r *http.Request) {
	target, ok := s.store.FindAppriseTarget(strings.TrimSpace(r.PathValue("id")))
	if !ok {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: "notification target not found"})
		return
	}
	service, ok := s.store.FindAppriseService(target.ServiceID)
	if !ok || service.Type != "openilink" {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: "target is not connected to OpeniLink Hub"})
		return
	}
	if !service.OpenILinkInboundEnabled {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: "OpeniLink inbound control is disabled"})
		return
	}
	result := s.notifier.UpdateOpenILinkToolsAt(r.Context(), service.BaseURL, time.Duration(service.NotifyTimeoutSeconds)*time.Second, target.ConfigKey, openILinkTools(service.OpenILinkCapabilities))
	if !result.OK {
		writeJSON(w, http.StatusBadGateway, model.APIResponse{Success: false, Error: result.Message})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: result})
}

func (s *Server) openILinkWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, openILinkWebhookMaxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
		return
	}

	var envelope openILinkEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if envelope.Type == "url_verification" {
		if strings.TrimSpace(envelope.Challenge) == "" {
			http.Error(w, "challenge is required", http.StatusBadRequest)
			return
		}
		writeOpenILinkJSON(w, http.StatusOK, map[string]string{"challenge": envelope.Challenge})
		return
	}

	service, err := s.authorizeOpenILinkWebhook(r, body, envelope.InstallationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if envelope.Type != "event" || envelope.Event.Type != "command" {
		http.Error(w, "only command events are supported", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(envelope.Event.ID) == "" {
		http.Error(w, "OpeniLink event ID is required", http.StatusBadRequest)
		return
	}

	var command openILinkCommand
	if err := json.Unmarshal(envelope.Event.Data, &command); err != nil {
		http.Error(w, "invalid command event", http.StatusBadRequest)
		return
	}
	command.Command = strings.ToLower(strings.TrimSpace(command.Command))
	eventID := strings.TrimSpace(envelope.Event.ID)
	cacheKey := envelope.InstallationID + ":" + eventID

	s.openILinkMu.Lock()
	defer s.openILinkMu.Unlock()
	s.pruneOpenILinkReplyCacheLocked(time.Now())
	if cached, ok := s.openILinkReplies[cacheKey]; ok {
		writeOpenILinkBytes(w, cached.status, cached.body)
		return
	}

	reply := s.executeOpenILinkCommand(service, command, cacheKey)
	responseBody, _ := json.Marshal(map[string]string{"reply": reply})
	s.openILinkReplies[cacheKey] = openILinkCachedReply{status: http.StatusOK, body: responseBody, createdAt: time.Now()}
	writeOpenILinkBytes(w, http.StatusOK, responseBody)
}

func (s *Server) authorizeOpenILinkWebhook(r *http.Request, body []byte, installationID string) (model.AppriseService, error) {
	timestampText := strings.TrimSpace(r.Header.Get("X-Timestamp"))
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil {
		return model.AppriseService{}, errors.New("invalid OpeniLink timestamp")
	}
	requestTime := time.Unix(timestamp, 0)
	if delta := time.Since(requestTime); delta > openILinkTimestampSkew || delta < -openILinkTimestampSkew {
		return model.AppriseService{}, errors.New("expired OpeniLink request")
	}
	signatureHeader := strings.TrimSpace(r.Header.Get("X-Signature"))
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return model.AppriseService{}, errors.New("invalid OpeniLink signature")
	}
	providedSignature, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, "sha256="))
	if err != nil || len(providedSignature) != sha256.Size {
		return model.AppriseService{}, errors.New("invalid OpeniLink signature")
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" || strings.TrimSpace(r.Header.Get("X-Installation-Id")) != installationID {
		return model.AppriseService{}, errors.New("OpeniLink installation ID mismatch")
	}

	signatureMatched := false
	for _, service := range s.store.AppriseServices() {
		if service.Type != "openilink" || !service.Enabled || !service.OpenILinkInboundEnabled {
			continue
		}
		mac := hmac.New(sha256.New, []byte(service.OpenILinkWebhookSecret))
		mac.Write([]byte(timestampText))
		mac.Write([]byte(":"))
		mac.Write(body)
		if !hmac.Equal(providedSignature, mac.Sum(nil)) {
			continue
		}
		signatureMatched = true
		if !containsExact(service.OpenILinkInstallationIDs, installationID) {
			continue
		}
		return service, nil
	}
	if signatureMatched {
		return model.AppriseService{}, errors.New("OpeniLink installation is not allowed")
	}
	return model.AppriseService{}, errors.New("OpeniLink signature is not recognized")
}

func (s *Server) executeOpenILinkCommand(service model.AppriseService, command openILinkCommand, sourceEventID string) string {
	if !containsExact(service.OpenILinkCapabilities, command.Command) {
		return fmt.Sprintf("SMS Hub 未启用功能 %s。", command.Command)
	}

	switch command.Command {
	case "get_overview":
		return marshalOpenILinkReply(s.store.Dashboard())
	case "list_devices":
		return marshalOpenILinkReply(s.store.Devices())
	case "search_sms":
		query := openILinkStringArg(command, "query")
		if query == "" {
			query = strings.TrimSpace(command.Text)
		}
		page := openILinkIntArg(command.Args, "page", 1)
		pageSize := openILinkIntArg(command.Args, "pageSize", 10)
		if pageSize > 20 {
			pageSize = 20
		}
		return marshalOpenILinkReply(s.store.SMS(query, page, pageSize))
	case "list_esim_profiles":
		deviceID := firstOpenILinkArg(command, "deviceId", 0)
		device, ok := s.store.FindDevice(deviceID)
		if !ok {
			return "设备不存在。"
		}
		profiles := make([]model.EsimProfile, 0)
		for _, profile := range s.store.EsimProfiles() {
			if profile.DeviceID == device.ID {
				profiles = append(profiles, profile)
			}
		}
		return marshalOpenILinkReply(map[string]interface{}{"device": device, "profiles": profiles})
	case "get_command_status":
		commandID := firstOpenILinkArg(command, "commandId", 0)
		for _, item := range s.store.Commands() {
			if item.ID == commandID {
				return marshalOpenILinkReply(item)
			}
		}
		return "命令不存在。"
	case "send_sms":
		deviceID := firstOpenILinkArg(command, "deviceId", 0)
		phone := firstOpenILinkArg(command, "phone", 1)
		body := openILinkStringArg(command, "body")
		if body == "" {
			parts := strings.Fields(command.Text)
			if len(parts) >= 3 {
				body = strings.Join(parts[2:], " ")
			}
		}
		if deviceID == "" || phone == "" || body == "" {
			return "用法：/send_sms <deviceId> <phone> <message>"
		}
		if len(phone) > 40 || len(body) > 2000 {
			return "手机号或短信内容过长。"
		}
		result, err := s.store.CreateSendSMSTaskFromSource(model.SendSMSRequest{DeviceID: deviceID, Phone: phone, Body: body}, sourceEventID)
		if err != nil {
			return "发送任务创建失败：" + err.Error()
		}
		return marshalOpenILinkReply(result)
	case "refresh_device_status":
		deviceID := firstOpenILinkArg(command, "deviceId", 0)
		if deviceID == "" {
			return "用法：/refresh_device_status <deviceId>"
		}
		created, err := s.store.CreateDeviceCommandFromSource(model.CreateDeviceCommandRequest{DeviceID: deviceID, Type: "query_status"}, sourceEventID)
		if err != nil {
			return "状态刷新任务创建失败：" + err.Error()
		}
		return marshalOpenILinkReply(created)
	case "switch_esim_profile":
		deviceID := firstOpenILinkArg(command, "deviceId", 0)
		iccid := firstOpenILinkArg(command, "iccid", 1)
		device, ok := s.store.FindDevice(deviceID)
		if !ok {
			return "设备不存在。"
		}
		if device.Status != "online" {
			return "设备离线，拒绝切换 eSIM Profile。"
		}
		profileFound := false
		for _, profile := range s.store.EsimProfiles() {
			if profile.DeviceID == device.ID && profile.ICCID == iccid {
				profileFound = true
				if profile.State == "enabled" || device.ICCID == iccid {
					return "该 eSIM Profile 已启用。"
				}
				break
			}
		}
		if !profileFound {
			return "该设备不存在指定的 eSIM Profile。"
		}
		created, err := s.store.CreateDeviceCommandFromSource(model.CreateDeviceCommandRequest{DeviceID: device.ID, Type: "esim_enable_profile", Payload: map[string]interface{}{"iccid": iccid}}, sourceEventID)
		if err != nil {
			return "eSIM 切换任务创建失败：" + err.Error()
		}
		return marshalOpenILinkReply(created)
	default:
		return fmt.Sprintf("SMS Hub 不支持命令 %s。", command.Command)
	}
}

func openILinkTools(capabilities []string) []notify.OpenILinkTool {
	tools := make([]notify.OpenILinkTool, 0, len(capabilities))
	for _, capability := range capabilities {
		tool, ok := openILinkToolDefinition(capability)
		if ok {
			tools = append(tools, tool)
		}
	}
	return tools
}

func openILinkToolDefinition(name string) (notify.OpenILinkTool, bool) {
	object := func(properties map[string]interface{}, required ...string) map[string]interface{} {
		result := map[string]interface{}{"type": "object", "properties": properties}
		if len(required) > 0 {
			result["required"] = required
		}
		return result
	}
	stringProperty := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": description}
	}
	integerProperty := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "integer", "description": description}
	}
	definitions := map[string]notify.OpenILinkTool{
		"get_overview":          {Name: "get_overview", Command: "get_overview", Description: "查看 SMS Hub 总览、终端在线数量、短信和任务状态", Parameters: object(map[string]interface{}{})},
		"list_devices":          {Name: "list_devices", Command: "list_devices", Description: "列出短信终端及在线、号码、运营商和信号状态", Parameters: object(map[string]interface{}{})},
		"search_sms":            {Name: "search_sms", Command: "search_sms", Description: "检索历史短信", Parameters: object(map[string]interface{}{"query": stringProperty("短信内容、发送方、接收方或消息 ID；可为空"), "page": integerProperty("页码"), "pageSize": integerProperty("每页数量，最多 20")})},
		"list_esim_profiles":    {Name: "list_esim_profiles", Command: "list_esim_profiles", Description: "列出指定终端的 eSIM Profile", Parameters: object(map[string]interface{}{"deviceId": stringProperty("终端 ID")}, "deviceId")},
		"get_command_status":    {Name: "get_command_status", Command: "get_command_status", Description: "查询 SMS Hub 命令执行状态", Parameters: object(map[string]interface{}{"commandId": stringProperty("命令 ID")}, "commandId")},
		"send_sms":              {Name: "send_sms", Command: "send_sms", Description: "通过指定终端发送短信，会产生运营商费用", Parameters: object(map[string]interface{}{"deviceId": stringProperty("终端 ID"), "phone": stringProperty("目标手机号"), "body": stringProperty("短信内容")}, "deviceId", "phone", "body")},
		"refresh_device_status": {Name: "refresh_device_status", Command: "refresh_device_status", Description: "让指定终端刷新状态", Parameters: object(map[string]interface{}{"deviceId": stringProperty("终端 ID")}, "deviceId")},
		"switch_esim_profile":   {Name: "switch_esim_profile", Command: "switch_esim_profile", Description: "切换指定终端的 eSIM Profile，会中断蜂窝连接", Parameters: object(map[string]interface{}{"deviceId": stringProperty("终端 ID"), "iccid": stringProperty("目标 Profile ICCID")}, "deviceId", "iccid")},
	}
	tool, ok := definitions[name]
	return tool, ok
}

func firstOpenILinkArg(command openILinkCommand, name string, textIndex int) string {
	if value := openILinkStringArg(command, name); value != "" {
		return value
	}
	fields := strings.Fields(command.Text)
	if textIndex >= 0 && textIndex < len(fields) {
		return fields[textIndex]
	}
	return ""
}

func openILinkStringArg(command openILinkCommand, name string) string {
	if command.Args == nil {
		return ""
	}
	value, ok := command.Args[name]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func openILinkIntArg(args map[string]interface{}, name string, fallback int) int {
	if args == nil {
		return fallback
	}
	value, ok := args[name].(float64)
	if !ok || value < 1 {
		return fallback
	}
	return int(value)
}

func marshalOpenILinkReply(value interface{}) string {
	body, err := json.Marshal(value)
	if err != nil {
		return "SMS Hub 无法生成结果。"
	}
	const maxReplyBytes = 12000
	if len(body) > maxReplyBytes {
		return string(body[:maxReplyBytes]) + "\n[结果过长，已截断]"
	}
	return string(body)
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Server) pruneOpenILinkReplyCacheLocked(now time.Time) {
	for key, reply := range s.openILinkReplies {
		if now.Sub(reply.createdAt) > openILinkReplyCacheTTL {
			delete(s.openILinkReplies, key)
		}
	}
}

func writeOpenILinkJSON(w http.ResponseWriter, status int, value interface{}) {
	body, _ := json.Marshal(value)
	writeOpenILinkBytes(w, status, body)
}

func writeOpenILinkBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
