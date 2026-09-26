package store

import (
	"strings"
	"testing"

	"sms-forwarding/server/api/internal/model"
)

func TestAppriseServiceNotificationTimeout(t *testing.T) {
	s := newEsimTaskTestStore(t)
	service, err := s.CreateAppriseService(model.CreateAppriseServiceRequest{
		Name: "Apprise", BaseURL: "http://apprise:8000", NotifyTimeoutSeconds: 30, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.NotifyTimeoutSeconds != 30 {
		t.Fatalf("timeout = %d, want 30", service.NotifyTimeoutSeconds)
	}

	updated, err := s.UpdateAppriseService(service.ID, model.UpdateAppriseServiceRequest{
		Name: "Apprise", BaseURL: "http://apprise:8000", NotifyTimeoutSeconds: 0, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.NotifyTimeoutSeconds != 15 {
		t.Fatalf("legacy/default timeout = %d, want 15", updated.NotifyTimeoutSeconds)
	}
}

func TestOpenILinkServiceAndTargetConfiguration(t *testing.T) {
	s := newEsimTaskTestStore(t)
	service, err := s.CreateAppriseService(model.CreateAppriseServiceRequest{
		Type: "openilink", Name: "WeChat", BaseURL: "http://openilink-hub:9800/", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.Type != "openilink" || service.BaseURL != "http://openilink-hub:9800" {
		t.Fatalf("service = %+v", service)
	}

	target, err := s.CreateAppriseTarget(model.CreateAppriseTargetRequest{
		ServiceID: service.ID, Name: "Family", ConfigKey: "app-token", Recipient: "user@im.wechat", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.Recipient != "user@im.wechat" {
		t.Fatalf("recipient = %q", target.Recipient)
	}
	if target.Description != "WeChat / OpeniLink / recipient: user@im.wechat" {
		t.Fatalf("description = %q", target.Description)
	}
	if strings.Contains(target.Description, "app-token") {
		t.Fatal("target description leaks the OpeniLink app token")
	}
}

func TestOpenILinkTargetRejectsDefaultToken(t *testing.T) {
	s := newEsimTaskTestStore(t)
	service, err := s.CreateAppriseService(model.CreateAppriseServiceRequest{
		Type: "openilink", Name: "WeChat", BaseURL: "http://openilink-hub:9800", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateAppriseTarget(model.CreateAppriseTargetRequest{
		ServiceID: service.ID, Name: "Invalid", ConfigKey: "default", Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "App Token") {
		t.Fatalf("create error = %v", err)
	}

	target, err := s.CreateAppriseTarget(model.CreateAppriseTargetRequest{
		ServiceID: service.ID, Name: "Valid", ConfigKey: "app-token", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateAppriseTarget(target.ID, model.CreateAppriseTargetRequest{
		ServiceID: service.ID, Name: "Invalid", ConfigKey: "DEFAULT", Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "App Token") {
		t.Fatalf("update error = %v", err)
	}
}

func TestOpenILinkInboundConfigurationRequiresSecurityFields(t *testing.T) {
	s := newEsimTaskTestStore(t)
	_, err := s.CreateAppriseService(model.CreateAppriseServiceRequest{
		Type: "openilink", Name: "WeChat", BaseURL: "http://openilink-hub:9800", Enabled: true,
		OpenILinkInboundEnabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "webhook secret") {
		t.Fatalf("error = %v", err)
	}

	service, err := s.CreateAppriseService(model.CreateAppriseServiceRequest{
		Type: "openilink", Name: "WeChat", BaseURL: "http://openilink-hub:9800", Enabled: true,
		OpenILinkInboundEnabled: true, OpenILinkWebhookSecret: "secret",
		OpenILinkInstallationIDs: []string{" inst-1 ", "inst-1"},
		OpenILinkCapabilities:    []string{"send_sms", "unknown", "get_overview"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(service.OpenILinkInstallationIDs) != 1 || len(service.OpenILinkCapabilities) != 2 {
		t.Fatalf("service = %+v", service)
	}
	if service.OpenILinkCapabilities[0] != "get_overview" || service.OpenILinkCapabilities[1] != "send_sms" {
		t.Fatalf("capabilities = %#v", service.OpenILinkCapabilities)
	}
}

func TestNormalizeNotifyTimeoutBounds(t *testing.T) {
	cases := map[int]int{0: 15, 1: 3, 3: 3, 15: 15, 120: 120, 121: 120}
	for input, wanted := range cases {
		if got := normalizeNotifyTimeout(input); got != wanted {
			t.Errorf("normalizeNotifyTimeout(%d) = %d, want %d", input, got, wanted)
		}
	}
}
