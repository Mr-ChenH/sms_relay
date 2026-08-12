package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/store"
)

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func TestDisabledServerReturnsNotFound(t *testing.T) {
	s := newTestStore(t)
	recorder := httptest.NewRecorder()
	New(s, Config{}).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestServerRequiresBearerToken(t *testing.T) {
	s := newTestStore(t)
	recorder := httptest.NewRecorder()
	New(s, Config{Token: "secret"}).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestMCPListsAndCallsReadOnlyTools(t *testing.T) {
	s := newTestStore(t)
	server := httptest.NewServer(New(s, Config{Token: "secret"}).Handler())
	defer server.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "sms-hub-test", Version: "1.0.0"}, nil)
	httpClient := &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: "secret"}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 8 {
		t.Fatalf("got %d tools, want 8", len(tools.Tools))
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_overview"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || result.StructuredContent == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSendSMSRequiresWriteOptIn(t *testing.T) {
	s := newTestStore(t)
	server := New(s, Config{Token: "secret"})
	_, _, err := server.sendSMS(context.Background(), nil, sendSMSInput{DeviceID: "device", Phone: "+8613800138000", Body: "test"})
	if err == nil {
		t.Fatal("expected write-disabled error")
	}
}

func TestSendSMSEnabledCreatesTask(t *testing.T) {
	s := newTestStore(t)
	_, err := s.RegisterTerminal(model.TerminalRegisterRequest{DeviceID: "terminal-1", Name: "Test terminal"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(s, Config{Token: "secret", AllowWrite: true})
	_, result, err := server.sendSMS(context.Background(), nil, sendSMSInput{DeviceID: "terminal-1", Phone: "+8613800138000", Body: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CommandID == "" || result.Status != "pending" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSwitchEsimProfileValidatesAndCreatesCommand(t *testing.T) {
	s := newTestStore(t)
	device, err := s.RegisterTerminal(model.TerminalRegisterRequest{DeviceID: "terminal-1", Name: "Test terminal"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReplaceTerminalEsimProfiles(model.TerminalEsimProfilesRequest{
		DeviceID: "terminal-1",
		Profiles: []model.TerminalEsimProfileInput{
			{ICCID: "8901000000000000001", ProfileName: "Current", State: "enabled"},
			{ICCID: "8901000000000000002", ProfileName: "Backup", State: "disabled"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	server := New(s, Config{Token: "secret", AllowWrite: true})
	_, profiles, err := server.listEsimProfiles(context.Background(), nil, deviceInput{DeviceID: device.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles.Profiles) != 2 {
		t.Fatalf("got %d profiles, want 2", len(profiles.Profiles))
	}

	_, _, err = server.switchEsimProfile(context.Background(), nil, switchProfileInput{DeviceID: device.ID, ICCID: "unknown"})
	if err == nil {
		t.Fatal("expected unknown ICCID error")
	}

	_, command, err := server.switchEsimProfile(context.Background(), nil, switchProfileInput{DeviceID: device.ID, ICCID: "8901000000000000002"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Type != "esim_enable_profile" || command.Payload["iccid"] != "8901000000000000002" {
		t.Fatalf("unexpected command: %#v", command)
	}

	_, found, err := server.getCommandStatus(context.Background(), nil, commandStatusInput{CommandID: command.ID})
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != command.ID || found.Status != "pending" {
		t.Fatalf("unexpected command status: %#v", found)
	}
}

func TestSwitchEsimProfileRequiresWriteOptIn(t *testing.T) {
	s := newTestStore(t)
	server := New(s, Config{Token: "secret"})
	_, _, err := server.switchEsimProfile(context.Background(), nil, switchProfileInput{DeviceID: "device", ICCID: "iccid"})
	if err == nil {
		t.Fatal("expected write-disabled error")
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "mcp-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return s
}
