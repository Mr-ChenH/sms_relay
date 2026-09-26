package notify

import (
	"context"
	"encoding/json"
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

func TestNotifyOpenILinkAtUsesAppriseEndpointAndBasicAuth(t *testing.T) {
	var got openILinkMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot/v1/apprise" || r.URL.Query().Get("to") != "user@im.wechat" {
			t.Errorf("request URL = %s", r.URL.String())
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "app-token" || password != "" {
			t.Errorf("basic auth = %q, %q, %v", username, password, ok)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("")
	result := client.NotifyOpenILinkAt(context.Background(), server.URL, time.Second, Message{
		Key: "app-token", Recipient: "user@im.wechat", Title: "SMS from 10086", Body: "hello", Type: "info",
	})
	if !result.OK || result.StatusCode != http.StatusOK {
		t.Fatalf("NotifyOpenILinkAt() = %+v", result)
	}
	if got.Version != "1.0" || got.Title != "SMS from 10086" || got.Message != "hello" || got.Type != "info" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestUpdateOpenILinkToolsAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/bot/v1/installation/tools" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer app-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Tools []OpenILinkTool `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Tools) != 1 || payload.Tools[0].Name != "send_sms" || payload.Tools[0].Command != "send_sms" {
			t.Errorf("tools = %#v", payload.Tools)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := NewClient("").UpdateOpenILinkToolsAt(context.Background(), server.URL, time.Second, "app-token", []OpenILinkTool{{
		Name: "send_sms", Command: "send_sms", Description: "send",
	}})
	if !result.OK || result.StatusCode != http.StatusOK {
		t.Fatalf("UpdateOpenILinkToolsAt() = %+v", result)
	}
}

func TestNotifyOpenILinkAtRequiresToken(t *testing.T) {
	result := NewClient("").NotifyOpenILinkAt(context.Background(), "http://example.test", time.Second, Message{Body: "hello"})
	if result.OK || result.Message != "OpeniLink app token is required" {
		t.Fatalf("NotifyOpenILinkAt() = %+v", result)
	}
}
