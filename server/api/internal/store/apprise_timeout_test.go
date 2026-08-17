package store

import (
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

func TestNormalizeNotifyTimeoutBounds(t *testing.T) {
	cases := map[int]int{0: 15, 1: 3, 3: 3, 15: 15, 120: 120, 121: 120}
	for input, wanted := range cases {
		if got := normalizeNotifyTimeout(input); got != wanted {
			t.Errorf("normalizeNotifyTimeout(%d) = %d, want %d", input, got, wanted)
		}
	}
}
