package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sms-forwarding/server/api/internal/model"

	_ "modernc.org/sqlite"
)

const (
	deviceOnlineTimeout = 45 * time.Second
	commandClaimTimeout = 2 * time.Minute
)

type Store struct {
	mu                sync.Mutex
	db                *sql.DB
	devices           []model.Device
	sms               []model.SMSMessage
	appriseServices   []model.AppriseService
	appriseTargets    []model.AppriseTarget
	rules             []model.RoutingRule
	esimProfiles      []model.EsimProfile
	esimTasks         []model.EsimTask
	esimSubscriptions []model.EsimSubscription
	keepaliveRuns     []model.EsimKeepaliveRun
	logs              []model.LogEntry
	audit             []model.AuditLog
	commands          []model.DeviceCommand
	nextID            int
	onCommandCreated  func(model.DeviceCommand, model.Device)
	onSMSStored       func(model.SMSMessage)
}

type snapshot struct {
	Devices           []model.Device           `json:"devices"`
	SMS               []model.SMSMessage       `json:"sms"`
	AppriseServices   []model.AppriseService   `json:"appriseServices"`
	AppriseTargets    []model.AppriseTarget    `json:"appriseTargets"`
	Rules             []model.RoutingRule      `json:"rules"`
	EsimProfiles      []model.EsimProfile      `json:"esimProfiles"`
	EsimTasks         []model.EsimTask         `json:"esimTasks"`
	EsimSubscriptions []model.EsimSubscription `json:"esimSubscriptions"`
	KeepaliveRuns     []model.EsimKeepaliveRun `json:"keepaliveRuns"`
	Logs              []model.LogEntry         `json:"logs"`
	Audit             []model.AuditLog         `json:"audit"`
	Commands          []model.DeviceCommand    `json:"commands"`
	NextID            int                      `json:"nextId"`
}

func NewSQLiteStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, nextID: 100}
	if err := store.initSQLite(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) initSQLite(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS app_state (id INTEGER PRIMARY KEY CHECK (id = 1), data TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		return err
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM app_state WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.persistLocked()
	}
	if err != nil {
		return err
	}
	var state snapshot
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return err
	}
	s.devices = state.Devices
	s.sms = state.SMS
	s.appriseServices = state.AppriseServices
	for i := range s.appriseServices {
		s.appriseServices[i].NotifyTimeoutSeconds = normalizeNotifyTimeout(s.appriseServices[i].NotifyTimeoutSeconds)
	}
	s.appriseTargets = state.AppriseTargets
	s.rules = state.Rules
	s.esimProfiles = state.EsimProfiles
	s.esimTasks = state.EsimTasks
	s.esimSubscriptions = state.EsimSubscriptions
	s.keepaliveRuns = state.KeepaliveRuns
	s.logs = state.Logs
	s.audit = state.Audit
	s.commands = state.Commands
	s.nextID = state.NextID
	for i := range s.esimTasks {
		task := &s.esimTasks[i]
		now := time.Now()
		if task.CreatedAt.IsZero() {
			task.CreatedAt = now
		}
		if task.UpdatedAt.IsZero() {
			task.UpdatedAt = task.CreatedAt
		}
		if len(task.History) == 0 && task.Stage != "" {
			task.History = []model.EsimTaskEvent{{Status: task.Status, Stage: task.Stage, Progress: task.Progress, CreatedAt: task.UpdatedAt}}
		}
		if task.Status == "pending" || task.Status == "running" {
			task.Status = "failed"
			task.Stage = "服务端重启，LPA 下载会话无法恢复"
			task.Progress = 0
			task.UpdatedAt = now
			task.History = append(task.History, model.EsimTaskEvent{Status: task.Status, Stage: task.Stage, Progress: task.Progress, CreatedAt: now})
		}
	}
	if s.nextID < 100 {
		s.nextID = 100
	}
	return nil
}

func (s *Store) SetCommandCreatedHook(hook func(model.DeviceCommand, model.Device)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCommandCreated = hook
}

func (s *Store) SetSMSStoredHook(hook func(model.SMSMessage)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSMSStored = hook
}

func (s *Store) persistLocked() error {
	if s.db == nil {
		return nil
	}
	state := snapshot{Devices: s.devices, SMS: s.sms, AppriseServices: s.appriseServices, AppriseTargets: s.appriseTargets, Rules: s.rules, EsimProfiles: s.esimProfiles, EsimTasks: s.esimTasks, EsimSubscriptions: s.esimSubscriptions, KeepaliveRuns: s.keepaliveRuns, Logs: s.logs, Audit: s.audit, Commands: s.commands, NextID: s.nextID}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO app_state (id, data, updated_at) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`, string(raw), time.Now().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Dashboard() model.Dashboard {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.refreshDeviceStatusesLocked(now)
	online := 0
	todaySMS := 0
	deliveryFailures := 0
	runningEsimTasks := 0
	today := now.Format("2006-01-02")
	for _, d := range s.devices {
		if deviceStatusAt(d, now) == "online" {
			online++
		}
	}
	for _, item := range s.sms {
		if item.Timestamp.Format("2006-01-02") == today {
			todaySMS++
		}
		if item.DeliveryStatus == "failed" || item.DeliveryStatus == "retrying" {
			deliveryFailures++
		}
	}
	for _, task := range s.esimTasks {
		if task.Status == "running" || task.Status == "pending" {
			runningEsimTasks++
		}
	}
	recentSMS := append([]model.SMSMessage{}, s.sms[:min(3, len(s.sms))]...)
	s.populateSMSRecipientsLocked(recentSMS)
	return model.Dashboard{
		OnlineDevices:     online,
		TotalDevices:      len(s.devices),
		TodaySMS:          todaySMS,
		DeliveryFailures:  deliveryFailures,
		RunningEsimTasks:  runningEsimTasks,
		RecentSMS:         recentSMS,
		Alerts:            []model.Alert{},
		EsimSubscriptions: append([]model.EsimSubscription{}, s.esimSubscriptions...),
	}
}

func (s *Store) Devices() []model.Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	changed := s.refreshDeviceStatusesLocked(now)
	if changed {
		defer s.persistLocked()
	}
	return append([]model.Device{}, s.devices...)
}

func (s *Store) FindDevice(id string) (model.Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findDeviceLocked(id)
}

func (s *Store) SMS(query string, page, pageSize int) model.SMSList {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := append([]model.SMSMessage{}, s.sms...)
	s.populateSMSRecipientsLocked(items)
	if query != "" {
		filtered := make([]model.SMSMessage, 0, len(items))
		q := strings.ToLower(query)
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Body), q) || strings.Contains(strings.ToLower(item.Sender), q) || strings.Contains(strings.ToLower(item.Recipient), q) || strings.Contains(strings.ToLower(item.ID), q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	earliest := time.Now()
	if len(s.sms) > 0 {
		earliest = s.sms[len(s.sms)-1].Timestamp
	}
	return model.SMSList{Items: items[start:end], Total: len(items), Page: page, PageSize: pageSize, TotalAll: len(s.sms), EarliestAt: earliest}
}

func (s *Store) populateSMSRecipientsLocked(items []model.SMSMessage) {
	for i := range items {
		if items[i].Recipient != "" {
			continue
		}
		if device, ok := s.findDeviceLocked(items[i].DeviceID); ok {
			items[i].Recipient = device.PhoneNumber
		}
	}
}

func (s *Store) AppriseServices() []model.AppriseService {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.AppriseService{}, s.appriseServices...)
}

func (s *Store) CreateAppriseService(req model.CreateAppriseServiceRequest) (model.AppriseService, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	name := strings.TrimSpace(req.Name)
	baseURL := normalizeBaseURL(req.BaseURL)
	if name == "" {
		return model.AppriseService{}, errors.New("name is required")
	}
	if baseURL == "" {
		return model.AppriseService{}, errors.New("baseUrl is required")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return model.AppriseService{}, errors.New("baseUrl must start with http:// or https://")
	}
	service := model.AppriseService{ID: s.nextIDStringLocked("apprise-service"), Name: name, BaseURL: baseURL, NotifyTimeoutSeconds: normalizeNotifyTimeout(req.NotifyTimeoutSeconds), Enabled: req.Enabled, LastStatus: "not_tested", LastMessage: "配置已保存，尚未测试连接", UpdatedAt: time.Now()}
	s.appriseServices = append(s.appriseServices, service)
	s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "create_apprise_service", ParameterSummary: service.Name + " / " + service.BaseURL, Result: "success", CreatedAt: time.Now()}}, s.audit...)
	return service, nil
}

func (s *Store) UpdateAppriseService(id string, req model.UpdateAppriseServiceRequest) (model.AppriseService, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	name := strings.TrimSpace(req.Name)
	baseURL := normalizeBaseURL(req.BaseURL)
	if name == "" {
		return model.AppriseService{}, errors.New("name is required")
	}
	if baseURL == "" {
		return model.AppriseService{}, errors.New("baseUrl is required")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return model.AppriseService{}, errors.New("baseUrl must start with http:// or https://")
	}
	for i := range s.appriseServices {
		if s.appriseServices[i].ID == id {
			s.appriseServices[i].Name = name
			s.appriseServices[i].BaseURL = baseURL
			s.appriseServices[i].NotifyTimeoutSeconds = normalizeNotifyTimeout(req.NotifyTimeoutSeconds)
			s.appriseServices[i].Enabled = req.Enabled
			s.appriseServices[i].LastStatus = "not_tested"
			s.appriseServices[i].LastMessage = "配置已保存，尚未测试连接"
			s.appriseServices[i].UpdatedAt = time.Now()
			for j := range s.appriseTargets {
				if s.appriseTargets[j].ServiceID == id {
					s.appriseTargets[j].ServiceName = name
				}
			}
			s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "update_apprise_service", ParameterSummary: name + " / " + baseURL, Result: "success", CreatedAt: time.Now()}}, s.audit...)
			return s.appriseServices[i], nil
		}
	}
	return model.AppriseService{}, errors.New("apprise service not found")
}

func (s *Store) DeleteAppriseService(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	for _, target := range s.appriseTargets {
		if target.ServiceID == id {
			return errors.New("apprise service is used by targets")
		}
	}
	for i, service := range s.appriseServices {
		if service.ID == id {
			s.appriseServices = append(s.appriseServices[:i], s.appriseServices[i+1:]...)
			s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "delete_apprise_service", ParameterSummary: service.Name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
			return nil
		}
	}
	return errors.New("apprise service not found")
}

func (s *Store) FindAppriseService(id string) (model.AppriseService, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findAppriseServiceLocked(id)
}

func (s *Store) UpdateAppriseServiceStatus(id, status, message string) (model.AppriseService, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	for i := range s.appriseServices {
		if s.appriseServices[i].ID == id {
			s.appriseServices[i].LastStatus = status
			s.appriseServices[i].LastMessage = message
			s.appriseServices[i].UpdatedAt = time.Now()
			return s.appriseServices[i], true
		}
	}
	return model.AppriseService{}, false
}

func (s *Store) DefaultAppriseService() (model.AppriseService, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.appriseServices) == 0 {
		return model.AppriseService{}, false
	}
	return s.appriseServices[0], true
}

func (s *Store) AppriseTargets() []model.AppriseTarget {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.AppriseTarget{}, s.appriseTargets...)
}

func (s *Store) CreateAppriseTarget(req model.CreateAppriseTargetRequest) (model.AppriseTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	name := strings.TrimSpace(req.Name)
	configKey := strings.TrimSpace(req.ConfigKey)
	if name == "" {
		return model.AppriseTarget{}, errors.New("name is required")
	}
	if configKey == "" {
		return model.AppriseTarget{}, errors.New("configKey is required")
	}

	serviceID := strings.TrimSpace(req.ServiceID)
	if serviceID == "" && len(s.appriseServices) > 0 {
		serviceID = s.appriseServices[0].ID
	}
	service, ok := s.findAppriseServiceLocked(serviceID)
	if !ok {
		return model.AppriseTarget{}, errors.New("apprise service is required")
	}
	tags := normalizeTags(req.Tags)
	target := model.AppriseTarget{
		ID:            s.nextIDStringLocked("apprise"),
		ServiceID:     service.ID,
		ServiceName:   service.Name,
		Name:          name,
		ConfigKey:     configKey,
		Tags:          tags,
		Enabled:       req.Enabled,
		TitleTemplate: firstNonEmpty(req.TitleTemplate, "短信来自 {{sender}}"),
		BodyTemplate:  firstNonEmpty(req.BodyTemplate, "{{body}}\n\n终端: {{device}}\n时间: {{timestamp}}"),
		LastStatus:    "not_tested",
		Description:   fmt.Sprintf("%s / key: %s / tag: %s", service.Name, configKey, firstNonEmpty(strings.Join(tags, ","), "all")),
	}
	s.appriseTargets = append(s.appriseTargets, target)
	s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "create_apprise_target", ParameterSummary: target.Name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
	return target, nil
}

func (s *Store) UpdateAppriseTarget(targetID string, req model.CreateAppriseTargetRequest) (model.AppriseTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	name := strings.TrimSpace(req.Name)
	configKey := strings.TrimSpace(req.ConfigKey)
	if name == "" {
		return model.AppriseTarget{}, errors.New("name is required")
	}
	if configKey == "" {
		return model.AppriseTarget{}, errors.New("configKey is required")
	}
	service, ok := s.findAppriseServiceLocked(strings.TrimSpace(req.ServiceID))
	if !ok {
		return model.AppriseTarget{}, errors.New("apprise service is required")
	}
	for i := range s.appriseTargets {
		if s.appriseTargets[i].ID == targetID {
			tags := normalizeTags(req.Tags)
			s.appriseTargets[i].ServiceID = service.ID
			s.appriseTargets[i].ServiceName = service.Name
			s.appriseTargets[i].Name = name
			s.appriseTargets[i].ConfigKey = configKey
			s.appriseTargets[i].Tags = tags
			s.appriseTargets[i].Enabled = req.Enabled
			s.appriseTargets[i].TitleTemplate = firstNonEmpty(req.TitleTemplate, "短信来自 {{sender}}")
			s.appriseTargets[i].BodyTemplate = firstNonEmpty(req.BodyTemplate, "{{body}}\n\n终端: {{device}}\n时间: {{timestamp}}")
			s.appriseTargets[i].Description = fmt.Sprintf("%s / key: %s / tag: %s", service.Name, configKey, firstNonEmpty(strings.Join(tags, ","), "all"))
			s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "update_apprise_target", ParameterSummary: name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
			return s.appriseTargets[i], nil
		}
	}
	return model.AppriseTarget{}, errors.New("apprise target not found")
}

func (s *Store) DeleteAppriseTarget(targetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	for i, target := range s.appriseTargets {
		if target.ID == targetID {
			s.appriseTargets = append(s.appriseTargets[:i], s.appriseTargets[i+1:]...)
			s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "delete_apprise_target", ParameterSummary: target.Name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
			return nil
		}
	}
	return errors.New("apprise target not found")
}

func (s *Store) EnabledAppriseTargets() []model.AppriseTarget {
	s.mu.Lock()
	defer s.mu.Unlock()
	targets := make([]model.AppriseTarget, 0, len(s.appriseTargets))
	for _, target := range s.appriseTargets {
		if target.Enabled {
			targets = append(targets, target)
		}
	}
	return targets
}

func (s *Store) UpdateAppriseTargetStatus(targetID, status, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	for i := range s.appriseTargets {
		if s.appriseTargets[i].ID == targetID {
			s.appriseTargets[i].LastStatus = status
			s.appriseTargets[i].Description = description
			return
		}
	}
}

func (s *Store) FindAppriseTarget(targetID string) (model.AppriseTarget, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, target := range s.appriseTargets {
		if target.ID == targetID {
			return target, true
		}
	}
	return model.AppriseTarget{}, false
}

func RenderAppriseMessage(target model.AppriseTarget, sms model.SMSMessage) (title, body, tag string) {
	titleTemplate := firstNonEmpty(target.TitleTemplate, "短信来自 {{sender}}")
	bodyTemplate := firstNonEmpty(target.BodyTemplate, "{{body}}")
	replacements := map[string]string{
		"{{sender}}":    sms.Sender,
		"{{body}}":      sms.Body,
		"{{device}}":    sms.DeviceName,
		"{{timestamp}}": formatLocalTimestamp(sms.Timestamp),
		"{{tag}}":       sms.Tag,
	}
	for placeholder, value := range replacements {
		titleTemplate = strings.ReplaceAll(titleTemplate, placeholder, value)
		bodyTemplate = strings.ReplaceAll(bodyTemplate, placeholder, value)
	}
	return titleTemplate, bodyTemplate, strings.Join(target.Tags, ",")
}

func formatLocalTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return ""
	}
	return timestamp.In(time.Local).Format(time.RFC3339)
}

func (s *Store) Rules() []model.RoutingRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.RoutingRule{}, s.rules...)
}

func (s *Store) CreateRoutingRule(req model.CreateRoutingRuleRequest) (model.RoutingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.RoutingRule{}, errors.New("name is required")
	}
	rule, err := s.routingRuleFromRequestLocked("", name, req)
	if err != nil {
		return model.RoutingRule{}, err
	}
	rule.ID = s.nextIDStringLocked("rule")
	s.rules = append(s.rules, rule)
	s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "create_routing_rule", ParameterSummary: rule.Name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
	return rule, nil
}

func (s *Store) UpdateRoutingRule(id string, req model.UpdateRoutingRuleRequest) (model.RoutingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.RoutingRule{}, errors.New("name is required")
	}
	for i := range s.rules {
		if s.rules[i].ID == id {
			rule, err := s.routingRuleFromRequestLocked(id, name, req)
			if err != nil {
				return model.RoutingRule{}, err
			}
			s.rules[i] = rule
			s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "update_routing_rule", ParameterSummary: name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
			return s.rules[i], nil
		}
	}
	return model.RoutingRule{}, errors.New("routing rule not found")
}

func (s *Store) routingRuleFromRequestLocked(id, name string, req model.CreateRoutingRuleRequest) (model.RoutingRule, error) {
	targetIDs := cleanStrings(req.TargetIDs)
	if len(targetIDs) == 0 {
		return model.RoutingRule{}, errors.New("at least one target is required")
	}
	for _, targetID := range targetIDs {
		found := false
		for _, target := range s.appriseTargets {
			if target.ID == targetID {
				found = true
				break
			}
		}
		if !found {
			return model.RoutingRule{}, fmt.Errorf("target not found: %s", targetID)
		}
	}
	return model.RoutingRule{
		ID: id, Name: name, SenderContains: strings.TrimSpace(req.SenderContains),
		BodyKeywords: cleanStrings(req.BodyKeywords), DeviceIDs: cleanStrings(req.DeviceIDs),
		Tags: cleanStrings(req.Tags), TargetIDs: targetIDs, Enabled: req.Enabled,
	}, nil
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *Store) RoutedAppriseTargets(sms model.SMSMessage) []model.AppriseTarget {
	s.mu.Lock()
	defer s.mu.Unlock()

	enabledTargets := make(map[string]model.AppriseTarget)
	for _, target := range s.appriseTargets {
		if target.Enabled {
			enabledTargets[target.ID] = target
		}
	}
	hasEnabledRule := false
	selected := make(map[string]struct{})
	for _, rule := range s.rules {
		if !rule.Enabled || len(rule.TargetIDs) == 0 {
			continue
		}
		hasEnabledRule = true
		if routingRuleMatches(rule, sms) {
			for _, targetID := range rule.TargetIDs {
				selected[targetID] = struct{}{}
			}
		}
	}

	result := make([]model.AppriseTarget, 0, len(enabledTargets))
	for _, target := range s.appriseTargets {
		if _, enabled := enabledTargets[target.ID]; !enabled {
			continue
		}
		if !hasEnabledRule {
			result = append(result, target)
			continue
		}
		if _, matched := selected[target.ID]; matched {
			result = append(result, target)
		}
	}
	return result
}

func routingRuleMatches(rule model.RoutingRule, sms model.SMSMessage) bool {
	if sender := strings.ToLower(strings.TrimSpace(rule.SenderContains)); sender != "" && !strings.Contains(strings.ToLower(sms.Sender), sender) {
		return false
	}
	if len(rule.BodyKeywords) > 0 {
		body := strings.ToLower(sms.Body)
		matched := false
		for _, keyword := range rule.BodyKeywords {
			if strings.Contains(body, strings.ToLower(keyword)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(rule.DeviceIDs) > 0 && !containsString(rule.DeviceIDs, sms.DeviceID) {
		return false
	}
	if len(rule.Tags) > 0 && !containsFold(rule.Tags, sms.Tag) {
		return false
	}
	return true
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func (s *Store) DeleteRoutingRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	for i, rule := range s.rules {
		if rule.ID == id {
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: "-", Action: "delete_routing_rule", ParameterSummary: rule.Name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
			return nil
		}
	}
	return errors.New("routing rule not found")
}

func (s *Store) EsimProfiles() []model.EsimProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.EsimProfile{}, s.esimProfiles...)
}

func (s *Store) UpdateEsimProfile(id string, req model.UpdateEsimProfileRequest) (model.EsimProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	for i := range s.esimProfiles {
		if s.esimProfiles[i].ID != id {
			continue
		}
		profile := &s.esimProfiles[i]
		profile.Country = strings.TrimSpace(req.Country)
		profile.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
		for j := range s.esimSubscriptions {
			if s.esimSubscriptions[j].ProfileID == profile.ID {
				s.esimSubscriptions[j].Country = profile.Country
				s.esimSubscriptions[j].UpdatedAt = time.Now()
			}
		}
		s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: profile.DeviceID, Action: "update_esim_profile", ParameterSummary: profile.ICCID, Result: "success", CreatedAt: time.Now()}}, s.audit...)
		return *profile, nil
	}
	return model.EsimProfile{}, errors.New("esim profile not found")
}

func (s *Store) ReplaceTerminalEsimProfiles(req model.TerminalEsimProfilesRequest) ([]model.EsimProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return nil, errors.New("device not found")
	}
	now := time.Now()
	seen := make(map[string]bool)
	profiles := make([]model.EsimProfile, 0, len(req.Profiles))
	for _, item := range req.Profiles {
		iccid := strings.TrimSpace(item.ICCID)
		aid := strings.TrimSpace(item.AID)
		if iccid == "" && aid == "" {
			continue
		}
		index := -1
		for i := range s.esimProfiles {
			if (iccid != "" && s.esimProfiles[i].ICCID == iccid) || (iccid == "" && aid != "" && s.esimProfiles[i].AID == aid) {
				index = i
				break
			}
		}
		if index < 0 {
			idPart := firstNonEmpty(iccid, aid)
			s.esimProfiles = append(s.esimProfiles, model.EsimProfile{ID: "profile-" + sanitizeIDPart(idPart)})
			index = len(s.esimProfiles) - 1
		}
		profile := &s.esimProfiles[index]
		profile.DeviceID = device.ID
		profile.ICCID = iccid
		profile.AID = aid
		profile.Nickname = strings.TrimSpace(item.Nickname)
		profile.Provider = strings.TrimSpace(item.Provider)
		if country := strings.TrimSpace(item.Country); country != "" {
			profile.Country = country
		}
		profile.ProfileName = strings.TrimSpace(item.ProfileName)
		profile.State = normalizeProfileState(item.State)
		profile.Available = true
		profile.MissingSince = time.Time{}
		profile.LastSeenAt = now
		seen[profile.ID] = true
		profiles = append(profiles, *profile)
	}
	for i := range s.esimProfiles {
		profile := &s.esimProfiles[i]
		if profile.DeviceID != device.ID || seen[profile.ID] {
			continue
		}
		profile.Available = false
		profile.State = "missing"
		if profile.MissingSince.IsZero() {
			profile.MissingSince = now
		}
	}
	for i := range s.esimSubscriptions {
		sub := &s.esimSubscriptions[i]
		profile, found := s.findEsimProfileLocked(sub.ProfileID)
		if !found {
			continue
		}
		if sub.DeviceID != profile.DeviceID {
			oldDevice := sub.DeviceID
			sub.DeviceID = profile.DeviceID
			if owner, found := s.findDeviceLocked(profile.DeviceID); found {
				sub.DeviceName = owner.Name
			}
			s.logs = append([]model.LogEntry{{ID: s.nextIDStringLocked("log"), DeviceID: profile.DeviceID, DeviceName: sub.DeviceName, Level: "info", Message: fmt.Sprintf("eSIM profile moved from %s to %s", oldDevice, profile.DeviceID), CreatedAt: now}}, s.logs...)
		}
		if !profile.Available {
			sub.Status = "profile_missing"
		} else if sub.Enabled && sub.Status == "profile_missing" {
			sub.Status = "scheduled"
		}
		sub.Country = profile.Country
		sub.UpdatedAt = now
	}
	s.logs = append([]model.LogEntry{{ID: s.nextIDStringLocked("log"), DeviceID: device.ID, DeviceName: device.Name, Level: "info", Message: fmt.Sprintf("eSIM profiles uploaded count=%d", len(profiles)), CreatedAt: now}}, s.logs...)
	return append([]model.EsimProfile{}, profiles...), nil
}

func (s *Store) EsimTasks() []model.EsimTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.EsimTask{}, s.esimTasks...)
}

func (s *Store) CreateEsimTask(req model.CreateEsimTaskRequest) (model.EsimTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return model.EsimTask{}, errors.New("device not found")
	}
	if deviceStatusAt(device, time.Now()) != "online" {
		return model.EsimTask{}, errors.New("terminal is offline")
	}
	activationCode := strings.TrimSpace(req.ActivationCode)
	if !strings.HasPrefix(strings.ToUpper(activationCode), "LPA:1$") {
		return model.EsimTask{}, errors.New("activationCode must use the LPA:1$ format")
	}
	for _, existing := range s.esimTasks {
		if existing.DeviceID == device.ID && (existing.Status == "pending" || existing.Status == "running") {
			return model.EsimTask{}, errors.New("another eSIM download is already running on this terminal")
		}
	}
	auditID := s.nextIDStringLocked("audit")
	now := time.Now()
	initialStage := "等待 LPA 启动"
	task := model.EsimTask{
		ID: s.nextIDStringLocked("esim-task"), DeviceID: device.ID, AuditID: auditID, Type: "download_profile",
		Status: "pending", Stage: initialStage, Progress: 0, CreatedAt: now, UpdatedAt: now,
		History: []model.EsimTaskEvent{{Status: "pending", Stage: initialStage, Progress: 0, CreatedAt: now}},
	}
	s.esimTasks = append([]model.EsimTask{task}, s.esimTasks...)
	s.audit = append([]model.AuditLog{{
		ID: auditID, Actor: "admin", DeviceName: device.Name,
		Action: "esim_download_profile", ParameterSummary: maskActivationCode(activationCode),
		Result: "pending", CreatedAt: now,
	}}, s.audit...)
	return task, nil
}

func (s *Store) UpdateEsimTask(id, status, stage string, progress int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	for i := range s.esimTasks {
		if s.esimTasks[i].ID != id {
			continue
		}
		task := &s.esimTasks[i]
		task.Status = status
		task.Stage = stage
		task.Progress = progress
		now := time.Now()
		task.UpdatedAt = now
		if len(task.History) == 0 || task.History[len(task.History)-1].Status != status || task.History[len(task.History)-1].Stage != stage || task.History[len(task.History)-1].Progress != progress {
			task.History = append(task.History, model.EsimTaskEvent{Status: status, Stage: stage, Progress: progress, CreatedAt: now})
		}
		for j := range s.audit {
			if s.audit[j].ID == s.esimTasks[i].AuditID {
				s.audit[j].Result = status
				break
			}
		}
		return nil
	}
	return errors.New("eSIM task not found")
}

func (s *Store) EsimSubscriptions() []model.EsimSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.EsimSubscription{}, s.esimSubscriptions...)
}

func (s *Store) CreateEsimSubscription(req model.CreateEsimSubscriptionRequest) (model.EsimSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	profile, ok := s.findEsimProfileLocked(req.ProfileID)
	if !ok {
		return model.EsimSubscription{}, errors.New("esim profile not found")
	}
	for _, sub := range s.esimSubscriptions {
		if sub.ProfileID == profile.ID {
			return model.EsimSubscription{}, errors.New("subscription already exists for profile")
		}
	}
	device, _ := s.findDeviceLocked(profile.DeviceID)
	now := time.Now()
	subType := normalizeSubscriptionType(req.Type)
	intervalDays := clampPositive(req.IntervalDays, 30)
	startAt := req.StartAt
	if startAt.IsZero() {
		startAt = now
	}
	targetIDs, err := s.validateSubscriptionTargetsLocked(req.TargetIDs)
	if err != nil {
		return model.EsimSubscription{}, err
	}
	sub := model.EsimSubscription{
		ID:               s.nextIDStringLocked("sub"),
		ProfileID:        profile.ID,
		ProfileName:      firstNonEmpty(profile.Nickname, profile.ProfileName),
		ICCID:            profile.ICCID,
		DeviceID:         profile.DeviceID,
		DeviceName:       firstNonEmpty(device.Name, profile.DeviceID),
		Country:          profile.Country,
		Enabled:          req.Enabled,
		Type:             subType,
		IntervalDays:     intervalDays,
		StartAt:          startAt,
		RechargeAmount:   strings.TrimSpace(req.RechargeAmount),
		KeepaliveNumber:  strings.TrimSpace(req.KeepaliveNumber),
		KeepaliveMessage: firstNonEmpty(req.KeepaliveMessage, "keepalive"),
		TargetIDs:        targetIDs,
		NextRunAt:        startAt,
		Status:           profileSubscriptionStatus(profile, req.Enabled),
		Note:             strings.TrimSpace(req.Note),
		UpdatedAt:        now,
	}
	s.esimSubscriptions = append(s.esimSubscriptions, sub)
	s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: sub.DeviceName, Action: "create_esim_subscription", ParameterSummary: sub.ProfileName, Result: "success", CreatedAt: now}}, s.audit...)
	return sub, nil
}

func (s *Store) UpdateEsimSubscription(id string, req model.UpdateEsimSubscriptionRequest) (model.EsimSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	for i := range s.esimSubscriptions {
		if s.esimSubscriptions[i].ID == id {
			targetIDs, err := s.validateSubscriptionTargetsLocked(req.TargetIDs)
			if err != nil {
				return model.EsimSubscription{}, err
			}
			now := time.Now()
			sub := &s.esimSubscriptions[i]
			sub.Enabled = req.Enabled
			sub.Type = normalizeSubscriptionType(req.Type)
			sub.IntervalDays = clampPositive(req.IntervalDays, 30)
			sub.StartAt = req.StartAt
			if sub.StartAt.IsZero() {
				sub.StartAt = now
			}
			sub.RechargeAmount = strings.TrimSpace(req.RechargeAmount)
			sub.KeepaliveNumber = strings.TrimSpace(req.KeepaliveNumber)
			sub.KeepaliveMessage = firstNonEmpty(req.KeepaliveMessage, "keepalive")
			sub.TargetIDs = targetIDs
			sub.NextRunAt = sub.StartAt
			sub.Status = "scheduled"
			if !sub.Enabled {
				sub.Status = "disabled"
			} else if profile, found := s.findEsimProfileLocked(sub.ProfileID); found && !profile.Available {
				sub.Status = "profile_missing"
			}
			sub.Note = strings.TrimSpace(req.Note)
			sub.UpdatedAt = now
			s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: sub.DeviceName, Action: "update_esim_subscription", ParameterSummary: sub.ProfileName, Result: "success", CreatedAt: now}}, s.audit...)
			return *sub, nil
		}
	}
	return model.EsimSubscription{}, errors.New("subscription not found")
}

func (s *Store) DeleteEsimSubscription(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	for _, run := range s.keepaliveRuns {
		if run.SubscriptionID == id && run.Stage != "completed" && run.Stage != "failed" {
			return errors.New("subscription has an active keepalive run")
		}
	}
	for i, sub := range s.esimSubscriptions {
		if sub.ID != id {
			continue
		}
		s.esimSubscriptions = append(s.esimSubscriptions[:i], s.esimSubscriptions[i+1:]...)
		s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: sub.DeviceName, Action: "delete_esim_subscription", ParameterSummary: sub.ProfileName, Result: "success", CreatedAt: time.Now()}}, s.audit...)
		return nil
	}
	return errors.New("subscription not found")
}

func (s *Store) SubscriptionAppriseTargets(targetIDs []string) []model.AppriseTarget {
	s.mu.Lock()
	defer s.mu.Unlock()

	selected := make(map[string]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		selected[id] = struct{}{}
	}
	targets := make([]model.AppriseTarget, 0, len(s.appriseTargets))
	for _, target := range s.appriseTargets {
		if !target.Enabled {
			continue
		}
		if len(selected) == 0 {
			targets = append(targets, target)
			continue
		}
		if _, ok := selected[target.ID]; ok {
			targets = append(targets, target)
		}
	}
	return targets
}

func (s *Store) validateSubscriptionTargetsLocked(values []string) ([]string, error) {
	targetIDs := cleanStrings(values)
	if len(targetIDs) == 0 {
		return nil, errors.New("at least one notification target is required")
	}
	for _, targetID := range targetIDs {
		found := false
		for _, target := range s.appriseTargets {
			if target.ID == targetID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("notification target not found: %s", targetID)
		}
	}
	return targetIDs, nil
}

func (s *Store) StartKeepaliveRun(sub model.EsimSubscription) (model.EsimKeepaliveRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	for _, run := range s.keepaliveRuns {
		if run.SubscriptionID == sub.ID && run.Stage != "completed" && run.Stage != "failed" {
			return run, nil
		}
	}
	device, ok := s.findDeviceLocked(sub.DeviceID)
	if !ok {
		return model.EsimKeepaliveRun{}, errors.New("device not found")
	}
	if strings.TrimSpace(sub.ICCID) == "" {
		return model.EsimKeepaliveRun{}, errors.New("target profile ICCID is required")
	}
	now := time.Now()
	run := model.EsimKeepaliveRun{
		ID: s.nextIDStringLocked("keepalive"), SubscriptionID: sub.ID, DeviceID: sub.DeviceID,
		TargetProfileID: sub.ProfileID, TargetICCID: sub.ICCID, OriginalICCID: strings.TrimSpace(device.ICCID),
		Phone: sub.KeepaliveNumber, Message: sub.KeepaliveMessage, TargetIDs: append([]string{}, sub.TargetIDs...),
		Stage: "pending", CreatedAt: now, UpdatedAt: now,
	}
	if run.OriginalICCID == "" {
		run.Stage = "failed"
		run.Error = "无法识别当前活跃 Profile，已拒绝自动切换"
	}
	s.keepaliveRuns = append(s.keepaliveRuns, run)
	return run, nil
}

func (s *Store) AdvanceKeepaliveRuns() []model.EsimKeepaliveRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	completed := make([]model.EsimKeepaliveRun, 0)
	for i := range s.keepaliveRuns {
		run := &s.keepaliveRuns[i]
		if run.Stage == "completed" || run.Stage == "failed" {
			if !run.NotificationSent {
				completed = append(completed, *run)
			}
			continue
		}
		device, ok := s.findDeviceLocked(run.DeviceID)
		if !ok {
			run.Stage, run.Error = "failed", "终端不存在"
			completed = append(completed, *run)
			continue
		}
		now := time.Now()
		switch run.Stage {
		case "pending":
			if run.OriginalICCID == run.TargetICCID {
				run.Stage = "ready_to_send"
			} else {
				cmd := s.createKeepaliveCommandLocked(device, "esim_enable_profile", map[string]interface{}{"iccid": run.TargetICCID, "keepaliveRunId": run.ID})
				run.CommandID, run.Stage = cmd.ID, "switching_to_target"
			}
		case "switching_to_target":
			cmd, found := s.findCommandLocked(run.CommandID)
			if found && commandFailed(cmd.Status) {
				run.Error = "切换到保活 Profile 失败：" + cmd.Result
				if device.ICCID != "" && device.ICCID != run.OriginalICCID {
					run.CommandID, run.Stage = "", "ready_to_restore"
				} else {
					run.Stage = "failed"
				}
			} else if device.ICCID == run.TargetICCID && found && commandSucceeded(cmd.Status) {
				run.CommandID, run.Stage = "", "ready_to_send"
			}
		case "ready_to_send":
			cmd := s.createKeepaliveCommandLocked(device, "send_sms", map[string]interface{}{"phone": run.Phone, "body": run.Message, "keepaliveRunId": run.ID})
			run.CommandID, run.Stage = cmd.ID, "sending_sms"
		case "sending_sms":
			cmd, found := s.findCommandLocked(run.CommandID)
			if !found || (!commandSucceeded(cmd.Status) && !commandFailed(cmd.Status)) {
				break
			}
			run.SMSResult = cmd.Result
			if commandFailed(cmd.Status) {
				run.Error = "保活短信发送失败：" + cmd.Result
			}
			if run.OriginalICCID != run.TargetICCID {
				run.CommandID, run.Stage = "", "ready_to_restore"
			} else if run.Error == "" {
				run.Stage = "completed"
			} else {
				run.Stage = "failed"
			}
		case "ready_to_restore":
			cmd := s.createKeepaliveCommandLocked(device, "esim_enable_profile", map[string]interface{}{"iccid": run.OriginalICCID, "keepaliveRunId": run.ID})
			run.CommandID, run.Stage = cmd.ID, "restoring_profile"
		case "restoring_profile":
			cmd, found := s.findCommandLocked(run.CommandID)
			if found && commandFailed(cmd.Status) {
				run.Stage = "failed"
				prefix := ""
				if run.Error != "" {
					prefix = run.Error + "；"
				}
				run.Error = prefix + "切回原 Profile 失败：" + cmd.Result
			} else if device.ICCID == run.OriginalICCID && found && commandSucceeded(cmd.Status) {
				if run.Error == "" {
					run.Stage = "completed"
				} else {
					run.Stage = "failed"
				}
			}
		}
		run.UpdatedAt = now
		if run.Stage == "completed" || run.Stage == "failed" {
			completed = append(completed, *run)
		}
	}
	return completed
}

func (s *Store) MarkKeepaliveRunNotified(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()
	for i := range s.keepaliveRuns {
		if s.keepaliveRuns[i].ID == id {
			s.keepaliveRuns[i].NotificationSent = true
			return
		}
	}
}

func (s *Store) createKeepaliveCommandLocked(device model.Device, commandType string, payload map[string]interface{}) model.DeviceCommand {
	cmd := model.DeviceCommand{ID: s.nextIDStringLocked("cmd"), DeviceID: device.ID, Type: commandType, Payload: payload, Status: "pending", CreatedAt: time.Now()}
	s.commands = append(s.commands, cmd)
	s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), CommandID: cmd.ID, Actor: "subscription", DeviceName: device.Name, Action: commandType, ParameterSummary: summarizePayload(payload), Result: "pending", CreatedAt: time.Now()}}, s.audit...)
	s.notifyCommandCreatedLocked(cmd, device)
	return cmd
}

func (s *Store) findCommandLocked(id string) (model.DeviceCommand, bool) {
	for _, cmd := range s.commands {
		if cmd.ID == id {
			return cmd, true
		}
	}
	return model.DeviceCommand{}, false
}

func commandSucceeded(status string) bool {
	return status == "succeeded" || status == "success"
}

func commandFailed(status string) bool {
	return status == "failed" || status == "error"
}

func (s *Store) ClaimDueEsimSubscriptions(now time.Time) []model.EsimSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()

	var due []model.EsimSubscription
	for i := range s.esimSubscriptions {
		sub := &s.esimSubscriptions[i]
		if sub.StartAt.IsZero() {
			sub.StartAt = sub.NextRunAt
		}
		if !sub.Enabled || sub.Status == "profile_missing" || sub.NextRunAt.IsZero() || sub.NextRunAt.After(now) {
			continue
		}
		due = append(due, *sub)
		sub.LastRunAt = now
		sub.NextRunAt = nextSubscriptionRun(sub.NextRunAt, sub.IntervalDays, now)
		sub.Status = "notifying"
		sub.UpdatedAt = now
	}
	if len(due) > 0 {
		_ = s.persistLocked()
	}
	return due
}

func (s *Store) CompleteEsimSubscriptionReminder(id string, sent bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.esimSubscriptions {
		if s.esimSubscriptions[i].ID != id {
			continue
		}
		if sent {
			s.esimSubscriptions[i].Status = "scheduled"
		} else {
			s.esimSubscriptions[i].Status = "notify_failed"
		}
		s.esimSubscriptions[i].UpdatedAt = time.Now()
		_ = s.persistLocked()
		return
	}
}

func nextSubscriptionRun(current time.Time, intervalDays int, now time.Time) time.Time {
	interval := time.Duration(clampPositive(intervalDays, 30)) * 24 * time.Hour
	next := current.Add(interval)
	for !next.After(now) {
		next = next.Add(interval)
	}
	return next
}

func (s *Store) Logs() []model.LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.LogEntry{}, s.logs...)
}

func (s *Store) Audit() []model.AuditLog {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Older persisted audit rows predate CommandID. Resolve those rows against
	// the corresponding command so completed work no longer appears pending.
	rows := append([]model.AuditLog{}, s.audit...)
	for i := range rows {
		if rows[i].Result != "pending" && rows[i].Result != "claimed" {
			continue
		}
		if command, ok := s.auditCommandLocked(rows[i]); ok {
			rows[i].CommandID = command.ID
			rows[i].Result = command.Status
		}
	}
	return rows
}

func (s *Store) auditCommandLocked(audit model.AuditLog) (model.DeviceCommand, bool) {
	if audit.CommandID != "" {
		return s.findCommandLocked(audit.CommandID)
	}
	for _, command := range s.commands {
		device, ok := s.findDeviceLocked(command.DeviceID)
		if !ok || device.Name != audit.DeviceName || command.Type != audit.Action {
			continue
		}
		if command.CreatedAt.Sub(audit.CreatedAt) < -time.Second || command.CreatedAt.Sub(audit.CreatedAt) > time.Second {
			continue
		}
		return command, true
	}
	return model.DeviceCommand{}, false
}

func (s *Store) updateCommandAuditLocked(commandID, status string) {
	for i := range s.audit {
		if s.audit[i].CommandID == commandID {
			s.audit[i].Result = status
			return
		}
	}
	command, ok := s.findCommandLocked(commandID)
	if !ok {
		return
	}
	for i := range s.audit {
		if s.audit[i].CommandID != "" || s.audit[i].Action != command.Type || (s.audit[i].Result != "pending" && s.audit[i].Result != "claimed") {
			continue
		}
		device, found := s.findDeviceLocked(command.DeviceID)
		if !found || s.audit[i].DeviceName != device.Name {
			continue
		}
		if command.CreatedAt.Sub(s.audit[i].CreatedAt) < -time.Second || command.CreatedAt.Sub(s.audit[i].CreatedAt) > time.Second {
			continue
		}
		s.audit[i].CommandID = commandID
		s.audit[i].Result = status
		return
	}
}

func (s *Store) Commands() []model.DeviceCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commands == nil {
		return []model.DeviceCommand{}
	}
	rows := append([]model.DeviceCommand{}, s.commands...)
	for i := range rows {
		if rows[i].Type != "firmware_update" || rows[i].Payload == nil {
			continue
		}
		payload := make(map[string]interface{}, len(rows[i].Payload))
		for key, value := range rows[i].Payload {
			payload[key] = value
		}
		if _, ok := payload["url"]; ok {
			payload["url"] = "[signed firmware URL]"
		}
		rows[i].Payload = payload
	}
	return rows
}

func (s *Store) CreateDeviceCommand(req model.CreateDeviceCommandRequest) (model.DeviceCommand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return model.DeviceCommand{}, errors.New("device not found")
	}
	cmdType := strings.TrimSpace(req.Type)
	if cmdType == "" {
		return model.DeviceCommand{}, errors.New("type is required")
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	cmd := model.DeviceCommand{ID: s.nextIDStringLocked("cmd"), DeviceID: device.ID, Type: cmdType, Payload: payload, Status: "pending", CreatedAt: time.Now()}
	s.commands = append(s.commands, cmd)
	s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), CommandID: cmd.ID, Actor: "admin", DeviceName: device.Name, Action: cmdType, ParameterSummary: summarizePayload(payload), Result: "pending", CreatedAt: time.Now()}}, s.audit...)
	s.notifyCommandCreatedLocked(cmd, device)
	return cmd, nil
}

func (s *Store) CreateSendSMSTask(req model.SendSMSRequest) (model.CommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return model.CommandResult{}, errors.New("device not found")
	}
	cmd := model.DeviceCommand{
		ID:        s.nextIDStringLocked("cmd"),
		DeviceID:  device.ID,
		Type:      "send_sms",
		Payload:   map[string]interface{}{"phone": req.Phone, "body": req.Body},
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	s.commands = append(s.commands, cmd)
	s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), CommandID: cmd.ID, Actor: "admin", DeviceName: device.Name, Action: "send_sms", ParameterSummary: maskPhone(req.Phone), Result: "pending", CreatedAt: time.Now()}}, s.audit...)
	s.notifyCommandCreatedLocked(cmd, device)
	return model.CommandResult{CommandID: cmd.ID, Status: cmd.Status, Message: "发送短信任务已创建，等待终端领取"}, nil
}

func (s *Store) UpdateDevice(id string, req model.UpdateDeviceRequest) (model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.Device{}, errors.New("name is required")
	}
	for i := range s.devices {
		if s.devices[i].ID != id && s.devices[i].DeviceID != id {
			continue
		}
		oldName := s.devices[i].Name
		s.devices[i].Name = name
		for j := range s.sms {
			if s.sms[j].DeviceID == s.devices[i].ID {
				s.sms[j].DeviceName = name
			}
		}
		for j := range s.logs {
			if s.logs[j].DeviceID == s.devices[i].ID {
				s.logs[j].DeviceName = name
			}
		}
		for j := range s.esimSubscriptions {
			if s.esimSubscriptions[j].DeviceID == s.devices[i].ID {
				s.esimSubscriptions[j].DeviceName = name
				s.esimSubscriptions[j].UpdatedAt = time.Now()
			}
		}
		for j := range s.audit {
			if s.audit[j].DeviceName == oldName {
				s.audit[j].DeviceName = name
			}
		}
		s.audit = append([]model.AuditLog{{ID: s.nextIDStringLocked("audit"), Actor: "admin", DeviceName: name, Action: "update_device_name", ParameterSummary: oldName + " -> " + name, Result: "success", CreatedAt: time.Now()}}, s.audit...)
		return s.devices[i], nil
	}
	return model.Device{}, errors.New("device not found")
}

func (s *Store) RegisterTerminal(req model.TerminalRegisterRequest) (model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	if strings.TrimSpace(req.DeviceID) == "" {
		return model.Device{}, errors.New("deviceId is required")
	}
	now := time.Now()
	if device, ok := s.findDeviceLocked(req.DeviceID); ok {
		wasOffline := device.Status != "online" || deviceStatusAt(device, now) == "offline"
		device.Status = "online"
		device.LastSeenAt = now
		device.FirmwareVersion = firstNonEmpty(req.FirmwareVersion, device.FirmwareVersion)
		device.HardwareModel = firstNonEmpty(req.HardwareModel, device.HardwareModel)
		device.IP = firstNonEmpty(strings.TrimSpace(req.IP), device.IP)
		if device.Name == "" || device.Name == device.DeviceID {
			device.Name = firstNonEmpty(req.Name, device.DeviceID)
		}
		s.upsertDeviceLocked(device)
		if wasOffline {
			s.appendDeviceStatusLogLocked(device, "online", now)
		}
		s.retryStaleCommandsLocked(device, now)
		s.notifyPendingCommandsLocked(device)
		return device, nil
	}
	name := firstNonEmpty(req.Name, req.DeviceID)
	device := model.Device{ID: s.nextIDStringLocked("dev"), DeviceID: req.DeviceID, Name: name, Status: "online", FirmwareVersion: req.FirmwareVersion, HardwareModel: req.HardwareModel, IP: strings.TrimSpace(req.IP), LastSeenAt: now}
	s.devices = append(s.devices, device)
	s.notifyPendingCommandsLocked(device)
	return device, nil
}

func (s *Store) Heartbeat(req model.TerminalHeartbeatRequest) (model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	now := time.Now()
	device, ok := s.findDeviceLocked(req.DeviceID)
	wasOffline := !ok || device.Status != "online" || deviceStatusAt(device, now) == "offline"
	if !ok {
		device = model.Device{ID: s.nextIDStringLocked("dev"), DeviceID: req.DeviceID, Name: req.DeviceID}
	}
	device.Status = "online"
	device.LastSeenAt = now
	device.FirmwareVersion = firstNonEmpty(req.FirmwareVersion, device.FirmwareVersion)
	device.HardwareModel = firstNonEmpty(req.HardwareModel, device.HardwareModel)
	newICCID := strings.TrimSpace(req.ICCID)
	if newICCID != "" && newICCID != device.ICCID {
		// 换卡：清空旧卡信息，等待终端查询并上报新卡号码/运营商
		device.ICCID = newICCID
		device.Operator = ""
		device.PhoneNumber = ""
	} else {
		// 终端启动初期或 eUICC 慢查询期间，心跳可能暂不携带号码/运营商，
		// 空值保留旧数据，避免覆盖为空白（终端就绪后会推送新值覆盖）。
		device.ICCID = firstNonEmpty(newICCID, device.ICCID)
		device.Operator = firstNonEmpty(strings.TrimSpace(req.Operator), device.Operator)
		device.PhoneNumber = firstNonEmpty(strings.TrimSpace(req.PhoneNumber), device.PhoneNumber)
	}
	device.EID = firstNonEmpty(req.EID, device.EID)
	device.EsimProfileVersion = firstNonEmpty(req.EsimProfileVersion, device.EsimProfileVersion)
	device.EsimSVN = firstNonEmpty(req.EsimSVN, device.EsimSVN)
	device.EsimFirmwareVersion = firstNonEmpty(req.EsimFirmwareVersion, device.EsimFirmwareVersion)
	device.EsimGlobalPlatformVersion = firstNonEmpty(req.EsimGlobalPlatformVersion, device.EsimGlobalPlatformVersion)
	device.EsimCategory = firstNonEmpty(req.EsimCategory, device.EsimCategory)
	device.EsimSASAccreditationNumber = firstNonEmpty(req.EsimSASAccreditationNumber, device.EsimSASAccreditationNumber)
	if req.EsimInstalledApplications > 0 {
		device.EsimInstalledApplications = req.EsimInstalledApplications
	}
	if req.EsimFreeNVMemory > 0 {
		device.EsimFreeNVMemory = req.EsimFreeNVMemory
	}
	if req.EsimFreeVolatileMemory > 0 {
		device.EsimFreeVolatileMemory = req.EsimFreeVolatileMemory
	}
	device.IP = firstNonEmpty(strings.TrimSpace(req.IP), device.IP)
	device.RSSI = req.RSSI
	device.CellularRSSI = req.CellularRSSI
	device.CellularCSQ = req.CellularCSQ
	device.FreeHeapKB = req.FreeHeapKB
	device.Uptime = firstNonEmpty(req.Uptime, device.Uptime)
	s.upsertDeviceLocked(device)
	if wasOffline {
		s.appendDeviceStatusLogLocked(device, "online", now)
	}
	s.updateEnabledProfileLocked(device)
	s.reconcileProfileCommandsLocked(device, now)
	s.retryStaleCommandsLocked(device, now)
	s.notifyPendingCommandsLocked(device)
	return device, nil
}

func (s *Store) MarkTerminalOnline(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	now := time.Now()
	device, ok := s.findDeviceLocked(deviceID)
	if !ok {
		device = model.Device{ID: s.nextIDStringLocked("dev"), DeviceID: deviceID, Name: deviceID}
	}
	wasOffline := device.Status != "online" || deviceStatusAt(device, now) == "offline"
	device.Status = "online"
	device.LastSeenAt = now
	s.upsertDeviceLocked(device)
	if wasOffline {
		s.appendDeviceStatusLogLocked(device, "online", now)
	}
	return nil
}

func (s *Store) MarkTerminalOffline(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(deviceID)
	if !ok {
		return errors.New("device not found")
	}
	if device.Status != "offline" {
		device.Status = "offline"
		s.upsertDeviceLocked(device)
		s.appendDeviceStatusLogLocked(device, "offline", time.Now())
	}
	return nil
}

func (s *Store) StoreTerminalSMS(req model.TerminalSMSRequest) (model.SMSMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return model.SMSMessage{}, false, errors.New("device not found")
	}
	for _, item := range s.sms {
		if item.DeviceID == device.ID && item.TerminalMessageID == req.TerminalMessageID && req.TerminalMessageID != "" {
			return item, false, nil
		}
	}
	timestamp := req.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	message := model.SMSMessage{ID: s.nextIDStringLocked("sms"), TerminalMessageID: req.TerminalMessageID, DeviceID: device.ID, DeviceName: device.Name, Sender: req.Sender, Recipient: firstNonEmpty(strings.TrimSpace(req.Recipient), device.PhoneNumber), Body: req.Body, Timestamp: timestamp, Tag: classifySMS(req.Sender, req.Body), DeliveryStatus: "success", DeliverySummary: "已入库", ConcatInfo: firstNonEmpty(req.ConcatInfo, "1/1")}
	s.sms = append([]model.SMSMessage{message}, s.sms...)
	s.logs = append([]model.LogEntry{{ID: s.nextIDStringLocked("log"), DeviceID: device.ID, DeviceName: device.Name, Level: "info", Message: fmt.Sprintf("SMS uploaded sender=%s len=%d", req.Sender, len([]rune(req.Body))), CreatedAt: time.Now()}}, s.logs...)
	if inserted := true; inserted {
		s.notifySMSStoredLocked(message)
	}
	return message, true, nil
}

func (s *Store) StoreTerminalLogs(req model.TerminalLogRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return errors.New("device not found")
	}
	for _, item := range req.Logs {
		level := firstNonEmpty(item.Level, "info")
		s.logs = append([]model.LogEntry{{ID: s.nextIDStringLocked("log"), DeviceID: device.ID, DeviceName: device.Name, Level: level, Message: item.Message, CreatedAt: time.Now()}}, s.logs...)
	}
	return nil
}

func (s *Store) UpdateCommandStatus(commandID string, req model.TerminalCommandResultRequest) (model.DeviceCommand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return model.DeviceCommand{}, errors.New("device not found")
	}
	now := time.Now()
	for i := range s.commands {
		if s.commands[i].ID == commandID && s.commands[i].DeviceID == device.ID {
			s.commands[i].Status = firstNonEmpty(req.Status, "claimed")
			if s.commands[i].Status == "claimed" && s.commands[i].ClaimedAt == nil {
				s.commands[i].ClaimedAt = &now
			}
			s.updateCommandAuditLocked(commandID, s.commands[i].Status)
			return s.commands[i], nil
		}
	}
	return model.DeviceCommand{}, errors.New("command not found")
}

func (s *Store) CompleteCommand(commandID string, req model.TerminalCommandResultRequest) (model.DeviceCommand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.persistLocked()

	device, ok := s.findDeviceLocked(req.DeviceID)
	if !ok {
		return model.DeviceCommand{}, errors.New("device not found")
	}
	now := time.Now()
	for i := range s.commands {
		if s.commands[i].ID == commandID && s.commands[i].DeviceID == device.ID {
			s.commands[i].Status = firstNonEmpty(req.Status, "succeeded")
			s.commands[i].Result = req.Result
			s.commands[i].CompletedAt = &now
			s.updateCommandAuditLocked(commandID, s.commands[i].Status)
			return s.commands[i], nil
		}
	}
	return model.DeviceCommand{}, errors.New("command not found")
}

func (s *Store) notifyCommandCreatedLocked(command model.DeviceCommand, device model.Device) {
	if s.onCommandCreated == nil {
		return
	}
	hook := s.onCommandCreated
	go hook(command, device)
}

func (s *Store) updateEnabledProfileLocked(device model.Device) bool {
	if device.ICCID == "" {
		return false
	}
	changed := false
	for i := range s.esimProfiles {
		profile := &s.esimProfiles[i]
		if profile.DeviceID != device.ID {
			continue
		}
		state := "disabled"
		if profile.ICCID == device.ICCID {
			state = "enabled"
		}
		if profile.State != state {
			profile.State = state
			changed = true
		}
	}
	return changed
}

func (s *Store) reconcileProfileCommandsLocked(device model.Device, now time.Time) bool {
	if device.ICCID == "" {
		return false
	}
	changed := false
	for i := range s.commands {
		cmd := &s.commands[i]
		if cmd.DeviceID != device.ID || cmd.Type != "esim_enable_profile" || (cmd.Status != "pending" && cmd.Status != "claimed") {
			continue
		}
		target, _ := cmd.Payload["iccid"].(string)
		if strings.TrimSpace(target) == device.ICCID {
			cmd.Status = "succeeded"
			cmd.Result = "profile enabled and verified by heartbeat"
			cmd.CompletedAt = &now
			s.updateCommandAuditLocked(cmd.ID, cmd.Status)
			changed = true
		}
	}
	return changed
}

func (s *Store) notifyPendingCommandsLocked(device model.Device) {
	for _, command := range s.commands {
		if command.DeviceID == device.ID && command.Status == "pending" {
			s.notifyCommandCreatedLocked(command, device)
		}
	}
}

func (s *Store) retryStaleCommandsLocked(device model.Device, now time.Time) bool {
	changed := false
	for i := range s.commands {
		cmd := &s.commands[i]
		if cmd.DeviceID != device.ID || cmd.Status != "claimed" || cmd.ClaimedAt == nil {
			continue
		}
		if now.Sub(*cmd.ClaimedAt) > commandClaimTimeout {
			cmd.Status = "pending"
			cmd.ClaimedAt = nil
			cmd.Result = ""
			changed = true
		}
	}
	return changed
}

func (s *Store) notifySMSStoredLocked(message model.SMSMessage) {
	if s.onSMSStored == nil {
		return
	}
	hook := s.onSMSStored
	go hook(message)
}

func (s *Store) refreshDeviceStatusesLocked(now time.Time) bool {
	changed := false
	for i := range s.devices {
		current := deviceStatusAt(s.devices[i], now)
		if s.devices[i].Status != current {
			s.devices[i].Status = current
			s.appendDeviceStatusLogLocked(s.devices[i], current, now)
			changed = true
		}
	}
	return changed
}

func (s *Store) appendDeviceStatusLogLocked(device model.Device, status string, now time.Time) {
	message := "terminal online"
	level := "info"
	if status == "offline" {
		message = "terminal offline"
		level = "warn"
		if !device.LastSeenAt.IsZero() {
			message += fmt.Sprintf("; last heartbeat %s ago", now.Sub(device.LastSeenAt).Round(time.Second))
		}
	}
	if device.IP != "" {
		message += "; ip=" + device.IP
	}
	if device.RSSI < 0 {
		message += fmt.Sprintf("; wifi=%d dBm", device.RSSI)
	}
	s.logs = append([]model.LogEntry{{ID: s.nextIDStringLocked("log"), DeviceID: device.ID, DeviceName: firstNonEmpty(device.Name, device.DeviceID), Level: level, Message: message, CreatedAt: now}}, s.logs...)
}

func devicesWithCurrentStatus(devices []model.Device, now time.Time) []model.Device {
	out := make([]model.Device, len(devices))
	for i, device := range devices {
		device.Status = deviceStatusAt(device, now)
		out[i] = device
	}
	return out
}

func deviceStatusAt(device model.Device, now time.Time) string {
	if device.LastSeenAt.IsZero() || now.Sub(device.LastSeenAt) > deviceOnlineTimeout {
		return "offline"
	}
	return "online"
}

func (s *Store) findDeviceLocked(id string) (model.Device, bool) {
	for _, device := range s.devices {
		if device.ID == id || device.DeviceID == id {
			return device, true
		}
	}
	return model.Device{}, false
}

func (s *Store) upsertDeviceLocked(device model.Device) {
	for i := range s.devices {
		if s.devices[i].ID == device.ID || s.devices[i].DeviceID == device.DeviceID {
			s.devices[i] = device
			return
		}
	}
	s.devices = append(s.devices, device)
}

func (s *Store) findEsimProfileLocked(id string) (model.EsimProfile, bool) {
	for _, profile := range s.esimProfiles {
		if profile.ID == id || profile.ICCID == id {
			return profile, true
		}
	}
	return model.EsimProfile{}, false
}

func (s *Store) findAppriseServiceLocked(id string) (model.AppriseService, bool) {
	for _, service := range s.appriseServices {
		if service.ID == id {
			return service, true
		}
	}
	return model.AppriseService{}, false
}

func (s *Store) nextIDStringLocked(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s-%d", prefix, s.nextID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func clampPositive(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func normalizeSubscriptionType(value string) string {
	switch strings.TrimSpace(value) {
	case "sms_keepalive":
		return "sms_keepalive"
	default:
		return "recharge"
	}
}

func normalizeNotifyTimeout(seconds int) int {
	if seconds == 0 {
		return 15
	}
	if seconds < 3 {
		return 3
	}
	if seconds > 120 {
		return 120
	}
	return seconds
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		for _, part := range strings.Split(tag, ",") {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	return out
}

func classifySMS(sender, body string) string {
	lower := strings.ToLower(body)
	if strings.Contains(body, "验证码") || strings.Contains(lower, "code") {
		return "验证码"
	}
	if sender == "10086" || sender == "10010" || sender == "10000" {
		return "运营商"
	}
	return "未分类"
}

func sanitizeIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func profileSubscriptionStatus(profile model.EsimProfile, enabled bool) string {
	if !enabled {
		return "disabled"
	}
	if !profile.Available {
		return "profile_missing"
	}
	return "scheduled"
}

func normalizeProfileState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "enabled", "enable", "active", "1":
		return "enabled"
	case "disabled", "disable", "inactive", "0":
		return "disabled"
	default:
		return "unknown"
	}
}

func maskActivationCode(code string) string {
	if len(code) <= 16 {
		return "LPA:***"
	}
	return code[:12] + "***" + code[len(code)-4:]
}

func summarizePayload(payload map[string]interface{}) string {
	parts := make([]string, 0, len(payload))
	for key, value := range payload {
		if key == "url" && strings.Contains(fmt.Sprint(value), "token=") {
			parts = append(parts, "url=[signed firmware URL]")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, ", ")
}

func maskPhone(phone string) string {
	if len(phone) <= 6 {
		return "***"
	}
	return phone[:3] + "****" + phone[len(phone)-3:]
}
