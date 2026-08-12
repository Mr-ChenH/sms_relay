package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"sms-forwarding/server/api/internal/httpapi"
	"sms-forwarding/server/api/internal/lpa"
	"sms-forwarding/server/api/internal/mcpserver"
	"sms-forwarding/server/api/internal/mqttbridge"
	"sms-forwarding/server/api/internal/mqttserver"
	"sms-forwarding/server/api/internal/notify"
	"sms-forwarding/server/api/internal/store"
)

func main() {
	addr := os.Getenv("SMS_HUB_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	appriseBaseURL := os.Getenv("APPRISE_BASE_URL")
	if appriseBaseURL == "" {
		appriseBaseURL = "http://localhost:8000"
	}

	dbPath := os.Getenv("SMS_HUB_DB_PATH")
	if dbPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		if filepath.Base(wd) == "api" {
			dbPath = filepath.Join("..", "data", "smshub.db")
		} else {
			dbPath = filepath.Join("data", "smshub.db")
		}
	}

	mqttBroker := os.Getenv("SMS_HUB_MQTT_BROKER")
	embeddedMQTT := os.Getenv("SMS_HUB_EMBEDDED_MQTT") != "false"
	embeddedMQTTAddr := os.Getenv("SMS_HUB_MQTT_ADDR")
	if embeddedMQTTAddr == "" {
		embeddedMQTTAddr = ":1883"
	}
	if mqttBroker == "" && embeddedMQTT {
		mqttBroker = "tcp://127.0.0.1:1883"
	}
	mqttClientID := os.Getenv("SMS_HUB_MQTT_CLIENT_ID")
	mqttUsername := os.Getenv("SMS_HUB_MQTT_USERNAME")
	mqttPassword := os.Getenv("SMS_HUB_MQTT_PASSWORD")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if embeddedMQTT {
		broker := mqttserver.New(embeddedMQTTAddr, s)
		if err := broker.Start(ctx); err != nil {
			log.Fatalf("embedded mqtt broker start failed: %v", err)
		}
	}

	bridge := mqttbridge.New(s, mqttBroker, mqttClientID, mqttUsername, mqttPassword)
	var lpaRunner *lpa.Runner
	if bridge.Enabled() {
		s.SetCommandCreatedHook(bridge.PublishCommand)
		bridge.Start(ctx)
		lpaRunner = lpa.NewRunner(bridge, s)
	}

	api := httpapi.New(s, notify.NewClient(appriseBaseURL), lpaRunner)
	mcpAPI := mcpserver.New(s, mcpserver.Config{
		Token:      os.Getenv("SMS_HUB_MCP_TOKEN"),
		AllowWrite: os.Getenv("SMS_HUB_MCP_ALLOW_WRITE") == "true",
	})
	rootHandler := api.Handler()
	if mcpAPI.Enabled() {
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcpAPI.Handler())
		mux.Handle("/", rootHandler)
		rootHandler = mux
		log.Printf("MCP enabled at /mcp (write=%t)", os.Getenv("SMS_HUB_MCP_ALLOW_WRITE") == "true")
	}
	go runSubscriptionReminders(ctx, s, notify.NewClient(appriseBaseURL))
	log.Printf("sms hub api listening on %s, apprise=%s, db=%s, mqtt=%s", addr, appriseBaseURL, dbPath, mqttBroker)
	if err := http.ListenAndServe(addr, rootHandler); err != nil {
		log.Fatal(err)
	}
}

func runSubscriptionReminders(ctx context.Context, s *store.Store, notifier *notify.Client) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	check := func() {
		for _, sub := range s.ClaimDueEsimSubscriptions(time.Now()) {
			if sub.Type == "sms_keepalive" {
				run, err := s.StartKeepaliveRun(sub)
				if err != nil {
					notifySubscription(ctx, s, notifier, sub.TargetIDs, "eSIM 保活失败", fmt.Sprintf("%s / %s 无法启动保活任务：%v", sub.DeviceName, sub.ProfileName, err))
					s.CompleteEsimSubscriptionReminder(sub.ID, false)
					continue
				}
				if run.Stage == "failed" {
					notified := notifySubscription(ctx, s, notifier, run.TargetIDs, "eSIM 保活失败", fmt.Sprintf("%s / %s：%s", sub.DeviceName, sub.ProfileName, run.Error))
					if notified {
						s.MarkKeepaliveRunNotified(run.ID)
					}
					s.CompleteEsimSubscriptionReminder(sub.ID, false)
				}
				continue
			}
			body := fmt.Sprintf("%s / %s 到达充值提醒时间。%s", sub.DeviceName, sub.ProfileName, sub.RechargeAmount)
			s.CompleteEsimSubscriptionReminder(sub.ID, notifySubscription(ctx, s, notifier, sub.TargetIDs, "eSIM 订阅提醒", body))
		}

		for _, run := range s.AdvanceKeepaliveRuns() {
			success := run.Stage == "completed"
			title := "eSIM 保活完成"
			body := fmt.Sprintf("保活短信已发送到 %s，并已恢复原 Profile %s。结果：%s", run.Phone, run.OriginalICCID, run.SMSResult)
			if run.OriginalICCID == run.TargetICCID {
				body = fmt.Sprintf("保活短信已通过当前 Profile 发送到 %s。结果：%s", run.Phone, run.SMSResult)
			}
			if !success {
				title = "eSIM 保活失败"
				body = run.Error
			}
			notified := notifySubscription(ctx, s, notifier, run.TargetIDs, title, body)
			if notified {
				s.MarkKeepaliveRunNotified(run.ID)
			}
			s.CompleteEsimSubscriptionReminder(run.SubscriptionID, success && notified)
		}
	}
	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

func notifySubscription(ctx context.Context, s *store.Store, notifier *notify.Client, targetIDs []string, title, body string) bool {
	notified := false
	for _, target := range s.SubscriptionAppriseTargets(targetIDs) {
		service, ok := s.FindAppriseService(target.ServiceID)
		if !ok || !service.Enabled {
			continue
		}
		result := notifier.NotifyAt(ctx, service.BaseURL, notify.Message{Key: target.ConfigKey, Tag: strings.Join(target.Tags, ","), Title: title, Body: body, Type: "info"})
		notified = notified || result.OK
		status := "failed"
		if result.OK {
			status = "success"
		}
		s.UpdateAppriseTargetStatus(target.ID, status, result.Message)
	}
	return notified
}
