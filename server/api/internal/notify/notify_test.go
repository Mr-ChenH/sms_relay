package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNotifyAllowsSlowUpstreamWithinConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("")
	result := client.NotifyAt(context.Background(), server.URL, 250*time.Millisecond, Message{Key: "test", Body: "hello"})
	if !result.OK || result.StatusCode != http.StatusOK {
		t.Fatalf("NotifyAt() = %+v", result)
	}
}

func TestNotifyStopsAtConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("")
	result := client.NotifyAt(context.Background(), server.URL, 30*time.Millisecond, Message{Key: "test", Body: "hello"})
	if result.OK || result.Message == "" {
		t.Fatalf("NotifyAt() = %+v, want timeout failure", result)
	}
}
