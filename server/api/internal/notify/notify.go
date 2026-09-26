package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 15 * time.Second
	defaultCheckTimeout   = 5 * time.Second
)

type Client struct {
	defaultBaseURL string
	http           *http.Client
}

type Message struct {
	Key       string `json:"-"`
	Recipient string `json:"-"`
	Tag       string `json:"tag,omitempty"`
	Title     string `json:"title,omitempty"`
	Body      string `json:"body"`
	Type      string `json:"type,omitempty"`
}

type OpenILinkTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Command     string                 `json:"command,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openILinkMessage struct {
	Version string `json:"version"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
}

type Result struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

func NewClient(defaultBaseURL string) *Client {
	return &Client{defaultBaseURL: normalizeBaseURL(defaultBaseURL), http: &http.Client{}}
}

func (c *Client) Notify(ctx context.Context, msg Message) Result {
	return c.NotifyAt(ctx, c.defaultBaseURL, defaultRequestTimeout, msg)
}

func (c *Client) NotifyAt(ctx context.Context, baseURL string, timeout time.Duration, msg Message) Result {
	baseURL = normalizeBaseURL(baseURL)
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	key := strings.TrimSpace(msg.Key)
	if key == "" {
		return Result{OK: false, Message: "apprise key is required"}
	}
	if strings.TrimSpace(msg.Body) == "" {
		return Result{OK: false, Message: "notification body is required"}
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}

	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint := baseURL + "/notify/" + url.PathEscape(key)
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return Result{OK: true, StatusCode: res.StatusCode, Message: "sent"}
	}
	return Result{OK: false, StatusCode: res.StatusCode, Message: fmt.Sprintf("apprise returned HTTP %d", res.StatusCode)}
}

func (c *Client) NotifyOpenILinkAt(ctx context.Context, baseURL string, timeout time.Duration, msg Message) Result {
	baseURL = normalizeBaseURL(baseURL)
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	token := strings.TrimSpace(msg.Key)
	if token == "" {
		return Result{OK: false, Message: "OpeniLink app token is required"}
	}
	if strings.TrimSpace(msg.Body) == "" {
		return Result{OK: false, Message: "notification body is required"}
	}

	payload, err := json.Marshal(openILinkMessage{Version: "1.0", Title: msg.Title, Message: msg.Body, Type: msg.Type})
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}

	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint := baseURL + "/bot/v1/apprise"
	if recipient := strings.TrimSpace(msg.Recipient); recipient != "" {
		endpoint += "?to=" + url.QueryEscape(recipient)
	}
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	req.SetBasicAuth(token, "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return Result{OK: true, StatusCode: res.StatusCode, Message: "sent"}
	}
	return Result{OK: false, StatusCode: res.StatusCode, Message: fmt.Sprintf("OpeniLink returned HTTP %d", res.StatusCode)}
}

func (c *Client) UpdateOpenILinkToolsAt(ctx context.Context, baseURL string, timeout time.Duration, token string, tools []OpenILinkTool) Result {
	baseURL = normalizeBaseURL(baseURL)
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Result{OK: false, Message: "OpeniLink app token is required"}
	}
	payload, err := json.Marshal(map[string]interface{}{"tools": tools})
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPut, baseURL+"/bot/v1/installation/tools", bytes.NewReader(payload))
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return Result{OK: true, StatusCode: res.StatusCode, Message: "tools synchronized"}
	}
	return Result{OK: false, StatusCode: res.StatusCode, Message: fmt.Sprintf("OpeniLink returned HTTP %d", res.StatusCode)}
}

func (c *Client) Check(ctx context.Context, baseURL string) Result {
	baseURL = normalizeBaseURL(baseURL)
	requestContext, cancel := context.WithTimeout(ctx, defaultCheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, baseURL+"/", nil)
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Result{OK: false, Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 400 {
		return Result{OK: true, StatusCode: res.StatusCode, Message: "connected"}
	}
	return Result{OK: false, StatusCode: res.StatusCode, Message: fmt.Sprintf("apprise returned HTTP %d", res.StatusCode)}
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "http://localhost:8000"
	}
	return baseURL
}
