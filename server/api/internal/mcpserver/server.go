package mcpserver

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/store"
)

const Version = "1.0.0"

type Config struct {
	Token      string
	AllowWrite bool
}

type Server struct {
	store   *store.Store
	config  Config
	handler http.Handler
}

type emptyInput struct{}

type searchSMSInput struct {
	Query    string `json:"query,omitempty" jsonschema:"Search SMS body, sender, recipient, or message ID"`
	Page     int    `json:"page,omitempty" jsonschema:"Page number starting at 1"`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"Results per page from 1 to 100"`
}

type sendSMSInput struct {
	DeviceID string `json:"deviceId" jsonschema:"required,Device ID returned by list_devices"`
	Phone    string `json:"phone" jsonschema:"required,Destination phone number including country code when applicable"`
	Body     string `json:"body" jsonschema:"required,SMS message body"`
}

type deviceInput struct {
	DeviceID string `json:"deviceId" jsonschema:"required,Device ID returned by list_devices"`
}

type commandStatusInput struct {
	CommandID string `json:"commandId" jsonschema:"required,Command ID returned by a control tool"`
}

type switchProfileInput struct {
	DeviceID string `json:"deviceId" jsonschema:"required,Device ID returned by list_devices"`
	ICCID    string `json:"iccid" jsonschema:"required,Target ICCID returned by list_esim_profiles"`
}

type profileListOutput struct {
	Device   model.Device        `json:"device"`
	Profiles []model.EsimProfile `json:"profiles"`
}

func New(s *store.Store, config Config) *Server {
	config.Token = strings.TrimSpace(config.Token)
	server := &Server{store: s, config: config}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "sms-hub", Version: Version}, nil)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "get_overview",
		Title:       "Get SMS Hub overview",
		Description: "Return terminal health, SMS traffic, delivery failures, running eSIM tasks, and recent SMS messages.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, server.getOverview)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_devices",
		Title:       "List SMS terminals",
		Description: "List registered SMS terminals with connectivity, SIM/eSIM, carrier, signal, and last-seen information.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, server.listDevices)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "search_sms",
		Title:       "Search historical SMS",
		Description: "Search stored SMS messages by body, sender, recipient, or message ID with pagination.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, server.searchSMS)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_esim_profiles",
		Title:       "List eSIM profiles",
		Description: "List known eSIM profiles for one terminal, including ICCID, provider, nickname, and enabled state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, server.listEsimProfiles)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "get_command_status",
		Title:       "Get command status",
		Description: "Look up the current state and terminal result for a command created by an MCP control tool.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, server.getCommandStatus)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "send_sms",
		Title:       "Send an SMS",
		Description: "Create an outbound SMS task for a terminal. This incurs carrier charges and requires SMS_HUB_MCP_ALLOW_WRITE=true.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: false},
	}, server.sendSMS)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "switch_esim_profile",
		Title:       "Switch active eSIM profile",
		Description: "Enable a known eSIM profile on a terminal by ICCID. This interrupts cellular connectivity and requires SMS_HUB_MCP_ALLOW_WRITE=true.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
	}, server.switchEsimProfile)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "refresh_device_status",
		Title:       "Refresh terminal status",
		Description: "Queue a standard status query for a terminal and return a command ID for polling.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
	}, server.refreshDeviceStatus)

	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{
		Stateless:           true,
		JSONResponse:        true,
		MaxRequestBodyBytes: 1 << 20,
	})
	server.handler = authenticate(config.Token, streamable)
	return server
}

func (s *Server) Enabled() bool {
	return s != nil && s.config.Token != ""
}

func (s *Server) Handler() http.Handler {
	if !s.Enabled() {
		return http.NotFoundHandler()
	}
	return s.handler
}

func (s *Server) getOverview(context.Context, *mcp.CallToolRequest, emptyInput) (*mcp.CallToolResult, model.Dashboard, error) {
	return nil, s.store.Dashboard(), nil
}

func (s *Server) listDevices(context.Context, *mcp.CallToolRequest, emptyInput) (*mcp.CallToolResult, []model.Device, error) {
	return nil, s.store.Devices(), nil
}

func (s *Server) searchSMS(_ context.Context, _ *mcp.CallToolRequest, input searchSMSInput) (*mcp.CallToolResult, model.SMSList, error) {
	page := input.Page
	if page < 1 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return nil, s.store.SMS(strings.TrimSpace(input.Query), page, pageSize), nil
}

func (s *Server) listEsimProfiles(_ context.Context, _ *mcp.CallToolRequest, input deviceInput) (*mcp.CallToolResult, profileListOutput, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	device, ok := s.store.FindDevice(deviceID)
	if !ok {
		return nil, profileListOutput{}, mcpError("device not found")
	}
	profiles := make([]model.EsimProfile, 0)
	for _, profile := range s.store.EsimProfiles() {
		if profile.DeviceID == device.ID {
			profiles = append(profiles, profile)
		}
	}
	return nil, profileListOutput{Device: device, Profiles: profiles}, nil
}

func (s *Server) getCommandStatus(_ context.Context, _ *mcp.CallToolRequest, input commandStatusInput) (*mcp.CallToolResult, model.DeviceCommand, error) {
	commandID := strings.TrimSpace(input.CommandID)
	if commandID == "" {
		return nil, model.DeviceCommand{}, mcpError("commandId is required")
	}
	for _, command := range s.store.Commands() {
		if command.ID == commandID {
			return nil, command, nil
		}
	}
	return nil, model.DeviceCommand{}, mcpError("command not found")
}

func (s *Server) sendSMS(_ context.Context, _ *mcp.CallToolRequest, input sendSMSInput) (*mcp.CallToolResult, model.CommandResult, error) {
	if !s.config.AllowWrite {
		return nil, model.CommandResult{}, mcpError("MCP write tools are disabled; set SMS_HUB_MCP_ALLOW_WRITE=true to enable send_sms")
	}
	return s.sendSMSResult(input)
}

func (s *Server) sendSMSResult(input sendSMSInput) (*mcp.CallToolResult, model.CommandResult, error) {
	deviceID := strings.TrimSpace(input.DeviceID)
	phone := strings.TrimSpace(input.Phone)
	body := strings.TrimSpace(input.Body)
	if deviceID == "" || phone == "" || body == "" {
		return nil, model.CommandResult{}, mcpError("deviceId, phone, and body are required")
	}
	result, err := s.store.CreateSendSMSTask(model.SendSMSRequest{DeviceID: deviceID, Phone: phone, Body: body})
	return nil, result, err
}

func (s *Server) switchEsimProfile(_ context.Context, _ *mcp.CallToolRequest, input switchProfileInput) (*mcp.CallToolResult, model.DeviceCommand, error) {
	if !s.config.AllowWrite {
		return nil, model.DeviceCommand{}, mcpError("MCP write tools are disabled; set SMS_HUB_MCP_ALLOW_WRITE=true to enable switch_esim_profile")
	}
	deviceID := strings.TrimSpace(input.DeviceID)
	iccid := strings.TrimSpace(input.ICCID)
	device, ok := s.store.FindDevice(deviceID)
	if !ok {
		return nil, model.DeviceCommand{}, mcpError("device not found")
	}
	if device.Status != "online" {
		return nil, model.DeviceCommand{}, mcpError("device is offline; refusing to switch profile")
	}
	profileFound := false
	for _, profile := range s.store.EsimProfiles() {
		if profile.DeviceID == device.ID && profile.ICCID == iccid {
			profileFound = true
			if profile.State == "enabled" || device.ICCID == iccid {
				return nil, model.DeviceCommand{}, mcpError("profile is already enabled")
			}
			break
		}
	}
	if !profileFound {
		return nil, model.DeviceCommand{}, mcpError("profile ICCID is not registered for this device")
	}
	command, err := s.store.CreateDeviceCommand(model.CreateDeviceCommandRequest{
		DeviceID: device.ID,
		Type:     "esim_enable_profile",
		Payload:  map[string]interface{}{"iccid": iccid},
	})
	return nil, command, err
}

func (s *Server) refreshDeviceStatus(_ context.Context, _ *mcp.CallToolRequest, input deviceInput) (*mcp.CallToolResult, model.DeviceCommand, error) {
	if !s.config.AllowWrite {
		return nil, model.DeviceCommand{}, mcpError("MCP write tools are disabled; set SMS_HUB_MCP_ALLOW_WRITE=true to enable refresh_device_status")
	}
	deviceID := strings.TrimSpace(input.DeviceID)
	if deviceID == "" {
		return nil, model.DeviceCommand{}, mcpError("deviceId is required")
	}
	command, err := s.store.CreateDeviceCommand(model.CreateDeviceCommandRequest{DeviceID: deviceID, Type: "query_status"})
	return nil, command, err
}

type mcpError string

func (e mcpError) Error() string { return string(e) }

func authenticate(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="sms-hub-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
