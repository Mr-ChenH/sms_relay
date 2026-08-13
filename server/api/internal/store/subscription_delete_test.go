package store

import (
	"testing"

	"sms-forwarding/server/api/internal/model"
)

func TestDeleteEsimSubscriptionKeepsCompletedRuns(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.esimSubscriptions = []model.EsimSubscription{{ID: "sub-1", ProfileName: "Travel", DeviceName: "Terminal"}}
	s.keepaliveRuns = []model.EsimKeepaliveRun{{ID: "run-1", SubscriptionID: "sub-1", Stage: "completed"}}

	if err := s.DeleteEsimSubscription("sub-1"); err != nil {
		t.Fatal(err)
	}
	if len(s.esimSubscriptions) != 0 {
		t.Fatalf("subscription was not deleted: %+v", s.esimSubscriptions)
	}
	if len(s.keepaliveRuns) != 1 {
		t.Fatalf("completed run history should be retained: %+v", s.keepaliveRuns)
	}
	if len(s.audit) != 1 || s.audit[0].Action != "delete_esim_subscription" {
		t.Fatalf("delete audit missing: %+v", s.audit)
	}
}

func TestDeleteEsimSubscriptionRejectsActiveRun(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.esimSubscriptions = []model.EsimSubscription{{ID: "sub-1"}}
	s.keepaliveRuns = []model.EsimKeepaliveRun{{ID: "run-1", SubscriptionID: "sub-1", Stage: "sending_sms"}}

	if err := s.DeleteEsimSubscription("sub-1"); err == nil {
		t.Fatal("expected active run to block deletion")
	}
	if len(s.esimSubscriptions) != 1 {
		t.Fatal("blocked subscription should remain")
	}
}
