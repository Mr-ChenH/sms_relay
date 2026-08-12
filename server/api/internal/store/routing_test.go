package store

import (
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestRoutedAppriseTargets(t *testing.T) {
	s := &Store{
		appriseTargets: []model.AppriseTarget{
			{ID: "target-verification", Name: "Verification", Enabled: true},
			{ID: "target-archive", Name: "Archive", Enabled: true},
			{ID: "target-disabled", Name: "Disabled", Enabled: false},
		},
	}
	sms := model.SMSMessage{DeviceID: "device-1", Sender: "95588", Body: "您的验证码是 123456", Tag: "verification", Timestamp: time.Now()}

	assertTargetIDs(t, s.RoutedAppriseTargets(sms), "target-verification", "target-archive")

	s.rules = []model.RoutingRule{
		{ID: "rule-1", Enabled: true, SenderContains: "955", BodyKeywords: []string{"验证码", "code"}, DeviceIDs: []string{"device-1"}, Tags: []string{"verification"}, TargetIDs: []string{"target-verification", "target-disabled"}},
		{ID: "rule-2", Enabled: true, BodyKeywords: []string{"验证码"}, TargetIDs: []string{"target-archive"}},
	}
	assertTargetIDs(t, s.RoutedAppriseTargets(sms), "target-verification", "target-archive")

	nonMatching := sms
	nonMatching.Body = "普通通知"
	nonMatching.Tag = "general"
	assertTargetIDs(t, s.RoutedAppriseTargets(nonMatching))
}

func TestRenderAppriseMessageUsesLocalTimezone(t *testing.T) {
	originalLocal := time.Local
	t.Cleanup(func() { time.Local = originalLocal })
	time.Local = time.FixedZone("UTC+8", 8*60*60)

	target := model.AppriseTarget{
		TitleTemplate: "短信来自 {{sender}}",
		BodyTemplate:  "{{body}}\n时间: {{timestamp}}",
		Tags:          []string{"verification"},
	}
	sms := model.SMSMessage{
		Sender:    "95588",
		Body:      "验证码 123456",
		Timestamp: time.Date(2026, time.August, 12, 7, 30, 0, 0, time.UTC),
	}

	title, body, tag := RenderAppriseMessage(target, sms)
	if title != "短信来自 95588" {
		t.Fatalf("title = %q", title)
	}
	if body != "验证码 123456\n时间: 2026-08-12T15:30:00+08:00" {
		t.Fatalf("body = %q", body)
	}
	if tag != "verification" {
		t.Fatalf("tag = %q", tag)
	}
}

func TestRoutingRuleMatchesUsesAndBetweenFieldsAndOrBetweenKeywords(t *testing.T) {
	rule := model.RoutingRule{
		SenderContains: "bank",
		BodyKeywords:   []string{"验证码", "code"},
		DeviceIDs:      []string{"device-1"},
		Tags:           []string{"verification"},
	}
	matching := model.SMSMessage{Sender: "MyBank", Body: "Your CODE is 42", DeviceID: "device-1", Tag: "Verification"}
	if !routingRuleMatches(rule, matching) {
		t.Fatal("expected rule to match")
	}
	matching.DeviceID = "device-2"
	if routingRuleMatches(rule, matching) {
		t.Fatal("expected device mismatch to reject message")
	}
}

func assertTargetIDs(t *testing.T, targets []model.AppriseTarget, wanted ...string) {
	t.Helper()
	if len(targets) != len(wanted) {
		t.Fatalf("got %d targets, want %d: %#v", len(targets), len(wanted), targets)
	}
	for i, target := range targets {
		if target.ID != wanted[i] {
			t.Fatalf("target[%d] = %q, want %q", i, target.ID, wanted[i])
		}
	}
}
