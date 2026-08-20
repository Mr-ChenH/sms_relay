package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestInferredBaseURLPreservesRequestPort(t *testing.T) {
	req := httptest.NewRequest("GET", "http://192.0.2.10:18082/api/admin/public-config", nil)
	if got := inferredBaseURL(req); got != "http://192.0.2.10:18082" {
		t.Fatalf("inferredBaseURL() = %q", got)
	}
}

func TestInferredBaseURLUsesForwardedHostAndProto(t *testing.T) {
	req := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/admin/public-config", nil)
	req.Header.Set("X-Forwarded-Host", "sms.example.test:8443")
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := inferredBaseURL(req); got != "https://sms.example.test:8443" {
		t.Fatalf("inferredBaseURL() = %q", got)
	}
}

func TestInferredBaseURLDefaultsTo8080WithoutPort(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.test/api/admin/public-config", nil)
	req.Host = "example.test"
	if got := inferredBaseURL(req); got != "http://example.test:8080" {
		t.Fatalf("inferredBaseURL() = %q", got)
	}
}
