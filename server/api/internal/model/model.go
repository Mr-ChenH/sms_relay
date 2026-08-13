package model

import "time"

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type Device struct {
	ID              string    `json:"id"`
	DeviceID        string    `json:"deviceId"`
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	FirmwareVersion string    `json:"firmwareVersion"`
	HardwareModel   string    `json:"hardwareModel"`
	ICCID           string    `json:"iccid"`
	EID             string    `json:"eid"`
	Operator        string    `json:"operator"`
	PhoneNumber     string    `json:"phoneNumber"`
	RSSI            int       `json:"rssi"`
	FreeHeapKB      int       `json:"freeHeapKb"`
	Uptime          string    `json:"uptime"`
	LastSeenAt      time.Time `json:"lastSeenAt"`
}

type UpdateDeviceRequest struct {
	Name string `json:"name"`
}

type SMSMessage struct {
	ID                string    `json:"id"`
	TerminalMessageID string    `json:"terminalMessageId"`
	DeviceID          string    `json:"deviceId"`
	DeviceName        string    `json:"deviceName"`
	Sender            string    `json:"sender"`
	Recipient         string    `json:"recipient"`
	Body              string    `json:"body"`
	Timestamp         time.Time `json:"timestamp"`
	Tag               string    `json:"tag"`
	DeliveryStatus    string    `json:"deliveryStatus"`
	DeliverySummary   string    `json:"deliverySummary"`
	ConcatInfo        string    `json:"concatInfo"`
}

type SMSList struct {
	Items      []SMSMessage `json:"items"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalAll   int          `json:"totalAll"`
	EarliestAt time.Time    `json:"earliestAt"`
}

type AppriseService struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	BaseURL     string    `json:"baseUrl"`
	Enabled     bool      `json:"enabled"`
	LastStatus  string    `json:"lastStatus"`
	LastMessage string    `json:"lastMessage"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateAppriseServiceRequest struct {
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Enabled bool   `json:"enabled"`
}

type UpdateAppriseServiceRequest = CreateAppriseServiceRequest

type AppriseTarget struct {
	ID            string   `json:"id"`
	ServiceID     string   `json:"serviceId"`
	ServiceName   string   `json:"serviceName"`
	Name          string   `json:"name"`
	ConfigKey     string   `json:"configKey"`
	Tags          []string `json:"tags"`
	Enabled       bool     `json:"enabled"`
	TitleTemplate string   `json:"titleTemplate"`
	BodyTemplate  string   `json:"bodyTemplate"`
	LastStatus    string   `json:"lastStatus"`
	Description   string   `json:"description"`
}

type CreateAppriseTargetRequest struct {
	ServiceID     string   `json:"serviceId"`
	Name          string   `json:"name"`
	ConfigKey     string   `json:"configKey"`
	Tags          []string `json:"tags"`
	Enabled       bool     `json:"enabled"`
	TitleTemplate string   `json:"titleTemplate"`
	BodyTemplate  string   `json:"bodyTemplate"`
}

type RoutingRule struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Conditions     string   `json:"conditions,omitempty"`
	Actions        string   `json:"actions,omitempty"`
	SenderContains string   `json:"senderContains,omitempty"`
	BodyKeywords   []string `json:"bodyKeywords,omitempty"`
	DeviceIDs      []string `json:"deviceIds,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	TargetIDs      []string `json:"targetIds,omitempty"`
	Enabled        bool     `json:"enabled"`
}

type CreateRoutingRuleRequest struct {
	Name           string   `json:"name"`
	SenderContains string   `json:"senderContains"`
	BodyKeywords   []string `json:"bodyKeywords"`
	DeviceIDs      []string `json:"deviceIds"`
	Tags           []string `json:"tags"`
	TargetIDs      []string `json:"targetIds"`
	Enabled        bool     `json:"enabled"`
}

type UpdateRoutingRuleRequest = CreateRoutingRuleRequest

type EsimProfile struct {
	ID           string    `json:"id"`
	DeviceID     string    `json:"deviceId"`
	ICCID        string    `json:"iccid"`
	AID          string    `json:"aid"`
	Nickname     string    `json:"nickname"`
	Provider     string    `json:"provider"`
	Country      string    `json:"country"`
	PhoneNumber  string    `json:"phoneNumber"`
	ProfileName  string    `json:"profileName"`
	State        string    `json:"state"`
	Available    bool      `json:"available"`
	MissingSince time.Time `json:"missingSince,omitempty"`
	LastSeenAt   time.Time `json:"lastSeenAt"`
}

type UpdateEsimProfileRequest struct {
	Country     string `json:"country"`
	PhoneNumber string `json:"phoneNumber"`
}

type EsimTaskEvent struct {
	Status    string    `json:"status"`
	Stage     string    `json:"stage"`
	Progress  int       `json:"progress"`
	CreatedAt time.Time `json:"createdAt"`
}

type EsimTask struct {
	ID        string          `json:"id"`
	DeviceID  string          `json:"deviceId"`
	AuditID   string          `json:"auditId,omitempty"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Stage     string          `json:"stage"`
	Progress  int             `json:"progress"`
	History   []EsimTaskEvent `json:"history"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type CreateEsimTaskRequest struct {
	DeviceID         string `json:"deviceId"`
	ActivationCode   string `json:"activationCode"`
	SMDPAddress      string `json:"smdpAddress"`
	ConfirmationCode string `json:"confirmationCode"`
}

type EsimSubscription struct {
	ID               string    `json:"id"`
	ProfileID        string    `json:"profileId"`
	ProfileName      string    `json:"profileName"`
	ICCID            string    `json:"iccid"`
	DeviceID         string    `json:"deviceId"`
	DeviceName       string    `json:"deviceName"`
	Country          string    `json:"country"`
	Enabled          bool      `json:"enabled"`
	Type             string    `json:"type"`
	IntervalDays     int       `json:"intervalDays"`
	StartAt          time.Time `json:"startAt"`
	RechargeAmount   string    `json:"rechargeAmount"`
	KeepaliveNumber  string    `json:"keepaliveNumber"`
	KeepaliveMessage string    `json:"keepaliveMessage"`
	TargetIDs        []string  `json:"targetIds"`
	NextRunAt        time.Time `json:"nextRunAt"`
	LastRunAt        time.Time `json:"lastRunAt"`
	Status           string    `json:"status"`
	Note             string    `json:"note"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type EsimKeepaliveRun struct {
	ID               string    `json:"id"`
	SubscriptionID   string    `json:"subscriptionId"`
	DeviceID         string    `json:"deviceId"`
	TargetProfileID  string    `json:"targetProfileId"`
	TargetICCID      string    `json:"targetIccid"`
	OriginalICCID    string    `json:"originalIccid"`
	Phone            string    `json:"phone"`
	Message          string    `json:"message"`
	TargetIDs        []string  `json:"targetIds"`
	Stage            string    `json:"stage"`
	CommandID        string    `json:"commandId"`
	SMSResult        string    `json:"smsResult"`
	Error            string    `json:"error"`
	NotificationSent bool      `json:"notificationSent"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CreateEsimSubscriptionRequest struct {
	ProfileID        string    `json:"profileId"`
	Enabled          bool      `json:"enabled"`
	Type             string    `json:"type"`
	IntervalDays     int       `json:"intervalDays"`
	StartAt          time.Time `json:"startAt"`
	RechargeAmount   string    `json:"rechargeAmount"`
	KeepaliveNumber  string    `json:"keepaliveNumber"`
	KeepaliveMessage string    `json:"keepaliveMessage"`
	TargetIDs        []string  `json:"targetIds"`
	Note             string    `json:"note"`
}

type UpdateEsimSubscriptionRequest struct {
	Enabled          bool      `json:"enabled"`
	Type             string    `json:"type"`
	IntervalDays     int       `json:"intervalDays"`
	StartAt          time.Time `json:"startAt"`
	RechargeAmount   string    `json:"rechargeAmount"`
	KeepaliveNumber  string    `json:"keepaliveNumber"`
	KeepaliveMessage string    `json:"keepaliveMessage"`
	TargetIDs        []string  `json:"targetIds"`
	Note             string    `json:"note"`
}

type LogEntry struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"deviceId"`
	DeviceName string    `json:"deviceName"`
	Level      string    `json:"level"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"createdAt"`
}

type AuditLog struct {
	ID               string    `json:"id"`
	CommandID        string    `json:"commandId,omitempty"`
	Actor            string    `json:"actor"`
	DeviceName       string    `json:"deviceName"`
	Action           string    `json:"action"`
	ParameterSummary string    `json:"parameterSummary"`
	Result           string    `json:"result"`
	CreatedAt        time.Time `json:"createdAt"`
}

type Dashboard struct {
	OnlineDevices     int                `json:"onlineDevices"`
	TotalDevices      int                `json:"totalDevices"`
	TodaySMS          int                `json:"todaySms"`
	DeliveryFailures  int                `json:"deliveryFailures"`
	RunningEsimTasks  int                `json:"runningEsimTasks"`
	RecentSMS         []SMSMessage       `json:"recentSms"`
	Alerts            []Alert            `json:"alerts"`
	EsimSubscriptions []EsimSubscription `json:"esimSubscriptions"`
}

type Alert struct {
	Time    string `json:"time"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

type SendSMSRequest struct {
	DeviceID string `json:"deviceId"`
	Phone    string `json:"phone"`
	Body     string `json:"body"`
}

type CommandResult struct {
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type NotifyTestRequest struct {
	TargetID string `json:"targetId"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type NotifyResult struct {
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

type DeviceCommand struct {
	ID          string                 `json:"id"`
	DeviceID    string                 `json:"deviceId"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
	Status      string                 `json:"status"`
	Result      string                 `json:"result,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	ClaimedAt   *time.Time             `json:"claimedAt,omitempty"`
	CompletedAt *time.Time             `json:"completedAt,omitempty"`
}

type CreateDeviceCommandRequest struct {
	DeviceID string                 `json:"deviceId"`
	Type     string                 `json:"type"`
	Payload  map[string]interface{} `json:"payload"`
}

type TerminalRegisterRequest struct {
	DeviceID        string `json:"deviceId"`
	Name            string `json:"name"`
	FirmwareVersion string `json:"firmwareVersion"`
	HardwareModel   string `json:"hardwareModel"`
}

type TerminalHeartbeatRequest struct {
	DeviceID        string `json:"deviceId"`
	FirmwareVersion string `json:"firmwareVersion"`
	HardwareModel   string `json:"hardwareModel"`
	ICCID           string `json:"iccid"`
	EID             string `json:"eid"`
	Operator        string `json:"operator"`
	PhoneNumber     string `json:"phoneNumber"`
	RSSI            int    `json:"rssi"`
	FreeHeapKB      int    `json:"freeHeapKb"`
	Uptime          string `json:"uptime"`
}

type TerminalSMSRequest struct {
	DeviceID          string    `json:"deviceId"`
	TerminalMessageID string    `json:"terminalMessageId"`
	Sender            string    `json:"sender"`
	Recipient         string    `json:"recipient"`
	Body              string    `json:"body"`
	Timestamp         time.Time `json:"timestamp"`
	ConcatInfo        string    `json:"concatInfo"`
}

type TerminalLogRequest struct {
	DeviceID string     `json:"deviceId"`
	Logs     []LogInput `json:"logs"`
}

type LogInput struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type TerminalEsimProfileInput struct {
	ICCID       string `json:"iccid"`
	AID         string `json:"aid"`
	Nickname    string `json:"nickname"`
	Provider    string `json:"provider"`
	Country     string `json:"country"`
	ProfileName string `json:"profileName"`
	State       string `json:"state"`
}

type TerminalEsimProfilesRequest struct {
	DeviceID string                     `json:"deviceId"`
	Profiles []TerminalEsimProfileInput `json:"profiles"`
}

type TerminalCommandResultRequest struct {
	DeviceID string `json:"deviceId"`
	Status   string `json:"status"`
	Result   string `json:"result"`
}
