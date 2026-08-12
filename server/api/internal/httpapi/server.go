package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"sms-forwarding/server/api/internal/lpa"
	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/notify"
	"sms-forwarding/server/api/internal/store"
	"sms-forwarding/server/api/internal/webui"
)

type Server struct {
	store            *store.Store
	notifier         *notify.Client
	publicBaseURL    string
	publicMQTTBroker string
	lpaRunner        *lpa.Runner
}

func New(s *store.Store, notifier *notify.Client, runners ...*lpa.Runner) *Server {
	server := &Server{store: s, notifier: notifier, publicBaseURL: strings.TrimSpace(os.Getenv("SMS_HUB_PUBLIC_BASE_URL")), publicMQTTBroker: strings.TrimSpace(os.Getenv("SMS_HUB_PUBLIC_MQTT_BROKER"))}
	if len(runners) > 0 {
		server.lpaRunner = runners[0]
	}
	s.SetSMSStoredHook(func(sms model.SMSMessage) {
		server.dispatchSMS(context.Background(), sms)
	})
	return server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)

	mux.HandleFunc("GET /api/admin/dashboard", s.dashboard)
	mux.HandleFunc("GET /api/admin/public-config", s.publicConfig)
	mux.HandleFunc("GET /api/admin/devices", s.devices)
	mux.HandleFunc("GET /api/admin/sms", s.sms)
	mux.HandleFunc("POST /api/admin/outbound-sms", s.sendSMS)
	mux.HandleFunc("GET /api/admin/commands", s.commands)
	mux.HandleFunc("POST /api/admin/commands", s.createCommand)
	mux.HandleFunc("GET /api/admin/apprise-service", s.appriseService)
	mux.HandleFunc("GET /api/admin/apprise-services", s.appriseServices)
	mux.HandleFunc("POST /api/admin/apprise-services", s.createAppriseService)
	mux.HandleFunc("PUT /api/admin/apprise-services/{id}", s.updateAppriseService)
	mux.HandleFunc("DELETE /api/admin/apprise-services/{id}", s.deleteAppriseService)
	mux.HandleFunc("POST /api/admin/apprise-services/test", s.testAppriseService)
	mux.HandleFunc("GET /api/admin/apprise-targets", s.appriseTargets)
	mux.HandleFunc("POST /api/admin/apprise-targets", s.createAppriseTarget)
	mux.HandleFunc("PUT /api/admin/apprise-targets/{id}", s.updateAppriseTarget)
	mux.HandleFunc("DELETE /api/admin/apprise-targets/{id}", s.deleteAppriseTarget)
	mux.HandleFunc("POST /api/admin/notify-test", s.notifyTest)
	mux.HandleFunc("GET /api/admin/routing-rules", s.rules)
	mux.HandleFunc("POST /api/admin/routing-rules", s.createRule)
	mux.HandleFunc("PUT /api/admin/routing-rules/{id}", s.updateRule)
	mux.HandleFunc("DELETE /api/admin/routing-rules/{id}", s.deleteRule)
	mux.HandleFunc("GET /api/admin/esim/profiles", s.esimProfiles)
	mux.HandleFunc("GET /api/admin/esim/capabilities", s.esimCapabilities)
	mux.HandleFunc("GET /api/admin/esim/tasks", s.esimTasks)
	mux.HandleFunc("POST /api/admin/esim/tasks", s.createEsimTask)
	mux.HandleFunc("GET /api/admin/esim/subscriptions", s.esimSubscriptions)
	mux.HandleFunc("POST /api/admin/esim/subscriptions", s.createEsimSubscription)
	mux.HandleFunc("PUT /api/admin/esim/subscriptions/{id}", s.updateEsimSubscription)
	mux.HandleFunc("GET /api/admin/logs", s.logs)
	mux.HandleFunc("GET /api/admin/audit", s.audit)

	webDir := strings.TrimSpace(os.Getenv("SMS_HUB_WEB_DIR"))
	if webDir != "" {
		if webHandler, ok := webui.Handler(webDir); ok {
			mux.Handle("/", webHandler)
		}
	}

	return cors(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: map[string]string{"status": "ok"}})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.Dashboard()})
}

func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	baseURL := s.publicBaseURL
	if baseURL == "" {
		baseURL = inferredBaseURL(r)
	}
	mqttBroker := s.publicMQTTBroker
	if mqttBroker == "" {
		mqttBroker = inferredMQTTBroker(r)
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: map[string]string{"apiBaseUrl": baseURL, "mqttBroker": mqttBroker}})
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.Devices()})
}

func (s *Server) sms(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.SMS(query, page, pageSize)})
}

func (s *Server) sendSMS(w http.ResponseWriter, r *http.Request) {
	var req model.SendSMSRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.DeviceID) == "" || strings.TrimSpace(req.Phone) == "" || strings.TrimSpace(req.Body) == "" {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: "deviceId, phone and body are required"})
		return
	}
	result, err := s.store.CreateSendSMSTask(req)
	if err != nil {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, model.APIResponse{Success: true, Data: result})
}

func (s *Server) commands(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.Commands()})
}

func (s *Server) createCommand(w http.ResponseWriter, r *http.Request) {
	var req model.CreateDeviceCommandRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cmd, err := s.store.CreateDeviceCommand(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, model.APIResponse{Success: true, Data: cmd})
}

func (s *Server) appriseService(w http.ResponseWriter, r *http.Request) {
	service, ok := s.store.DefaultAppriseService()
	if !ok {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: "apprise service not found"})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: service})
}

func (s *Server) appriseServices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.AppriseServices()})
}

func (s *Server) createAppriseService(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAppriseServiceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	service, err := s.store.CreateAppriseService(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, model.APIResponse{Success: true, Data: service})
}

func (s *Server) updateAppriseService(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req model.UpdateAppriseServiceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	service, err := s.store.UpdateAppriseService(id, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: service})
}

func (s *Server) deleteAppriseService(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.store.DeleteAppriseService(id); err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: map[string]string{"status": "deleted"}})
}

func (s *Server) testAppriseService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceID string `json:"serviceId"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	service, ok := s.store.FindAppriseService(req.ServiceID)
	if !ok {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: "apprise service not found"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	result := s.notifier.Check(ctx, service.BaseURL)
	status := "success"
	if !result.OK {
		status = "failed"
	}
	updated, _ := s.store.UpdateAppriseServiceStatus(service.ID, status, result.Message)
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: map[string]interface{}{"service": updated, "result": result}})
}

func (s *Server) appriseTargets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.AppriseTargets()})
}

func (s *Server) createAppriseTarget(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAppriseTargetRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	target, err := s.store.CreateAppriseTarget(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, model.APIResponse{Success: true, Data: target})
}

func (s *Server) updateAppriseTarget(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req model.CreateAppriseTargetRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	target, err := s.store.UpdateAppriseTarget(id, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: target})
}

func (s *Server) deleteAppriseTarget(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.store.DeleteAppriseTarget(id); err != nil {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: map[string]string{"status": "deleted"}})
}

func (s *Server) notifyTest(w http.ResponseWriter, r *http.Request) {
	var req model.NotifyTestRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	target, ok := s.store.FindAppriseTarget(req.TargetID)
	if !ok {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: "apprise target not found"})
		return
	}
	service, ok := s.store.FindAppriseService(target.ServiceID)
	if !ok {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: "apprise service not found"})
		return
	}
	if !service.Enabled {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: "apprise service is disabled"})
		return
	}
	body := firstNonEmpty(req.Body, "SMS Hub Apprise test")
	title := firstNonEmpty(req.Title, "SMS Hub 测试通知")
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	result := s.notifier.NotifyAt(ctx, service.BaseURL, notify.Message{Key: target.ConfigKey, Tag: strings.Join(target.Tags, ","), Title: title, Body: body, Type: "info"})
	status := "success"
	if !result.OK {
		status = "failed"
	}
	s.store.UpdateAppriseTargetStatus(target.ID, status, result.Message)
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: model.NotifyResult{TargetID: target.ID, TargetName: target.Name, OK: result.OK, StatusCode: result.StatusCode, Message: result.Message}})
}

func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.Rules()})
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var req model.CreateRoutingRuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rule, err := s.store.CreateRoutingRule(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, model.APIResponse{Success: true, Data: rule})
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req model.UpdateRoutingRuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rule, err := s.store.UpdateRoutingRule(id, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: rule})
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.store.DeleteRoutingRule(id); err != nil {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: map[string]string{"status": "deleted"}})
}

func (s *Server) esimCapabilities(w http.ResponseWriter, r *http.Request) {
	supported := false
	reason := "LPA runner is not configured"
	if s.lpaRunner != nil {
		if err := s.lpaRunner.Available(); err != nil {
			reason = err.Error()
		} else {
			supported = true
			reason = ""
		}
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: map[string]interface{}{
		"profileDownload": supported,
		"platform":        runtime.GOOS,
		"reason":          reason,
	}})
}

func (s *Server) esimProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.EsimProfiles()})
}

func (s *Server) esimTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.EsimTasks()})
}

func (s *Server) createEsimTask(w http.ResponseWriter, r *http.Request) {
	var req model.CreateEsimTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.lpaRunner == nil {
		writeJSON(w, http.StatusServiceUnavailable, model.APIResponse{Success: false, Error: "LPA runner is not configured"})
		return
	}
	if err := s.lpaRunner.Available(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	task, err := s.store.CreateEsimTask(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	device, ok := s.store.FindDevice(task.DeviceID)
	if !ok || strings.TrimSpace(device.DeviceID) == "" {
		_ = s.store.UpdateEsimTask(task.ID, "failed", "无法解析终端 MQTT ID", 0)
		writeJSON(w, http.StatusServiceUnavailable, model.APIResponse{Success: false, Error: "terminal MQTT ID is unavailable"})
		return
	}
	if err := s.lpaRunner.Start(task.ID, device.DeviceID, strings.TrimSpace(req.ActivationCode), strings.TrimSpace(req.ConfirmationCode)); err != nil {
		_ = s.store.UpdateEsimTask(task.ID, "failed", "无法启动 LPA："+err.Error(), 0)
		writeJSON(w, http.StatusServiceUnavailable, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, model.APIResponse{Success: true, Data: task})
}

func (s *Server) esimSubscriptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.EsimSubscriptions()})
}

func (s *Server) createEsimSubscription(w http.ResponseWriter, r *http.Request) {
	var req model.CreateEsimSubscriptionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, err := s.store.CreateEsimSubscription(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, model.APIResponse{Success: true, Data: sub})
}

func (s *Server) updateEsimSubscription(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req model.UpdateEsimSubscriptionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, err := s.store.UpdateEsimSubscription(id, req)
	if err != nil {
		writeJSON(w, http.StatusNotFound, model.APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: sub})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.Logs()})
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.APIResponse{Success: true, Data: s.store.Audit()})
}

func (s *Server) dispatchSMS(parent context.Context, sms model.SMSMessage) []model.NotifyResult {
	targets := s.store.RoutedAppriseTargets(sms)
	results := make([]model.NotifyResult, 0, len(targets))
	for _, target := range targets {
		service, ok := s.store.FindAppriseService(target.ServiceID)
		if !ok || !service.Enabled {
			continue
		}
		title, body, tag := store.RenderAppriseMessage(target, sms)
		ctx, cancel := context.WithTimeout(parent, 6*time.Second)
		result := s.notifier.NotifyAt(ctx, service.BaseURL, notify.Message{Key: target.ConfigKey, Tag: tag, Title: title, Body: body, Type: "info"})
		cancel()
		status := "success"
		if !result.OK {
			status = "failed"
		}
		s.store.UpdateAppriseTargetStatus(target.ID, status, result.Message)
		results = append(results, model.NotifyResult{TargetID: target.ID, TargetName: target.Name, OK: result.OK, StatusCode: result.StatusCode, Message: result.Message})
	}
	return results
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func inferredBaseURL(r *http.Request) string {
	host := requestHost(r)
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + host + ":8080"
}

func inferredMQTTBroker(r *http.Request) string {
	return "mqtt://" + requestHost(r) + ":1883"
}

func requestHost(r *http.Request) string {
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if strings.Contains(host, ",") {
		host = strings.TrimSpace(strings.Split(host, ",")[0])
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.Count(host, ":") == 0 {
		host = strings.TrimSpace(host)
	}
	return strings.Trim(host, "[]")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		writeJSON(w, http.StatusBadRequest, model.APIResponse{Success: false, Error: "invalid JSON body"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value model.APIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
