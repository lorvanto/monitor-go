package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestChatWebhookSignaturesAndPayloads(t *testing.T) {
	p := notificationPayload{DeliveryID: "42", Event: "opened", At: 1700000000000, Resource: "Production API", Severity: "critical", Title: "原始标题", Description: "Original diagnostic"}
	for _, format := range []string{"generic", "dingtalk", "feishu", "wecom"} {
		t.Run(format, func(t *testing.T) {
			req, err := webhookRequest(context.Background(), channelConfig{URL: "https://example.com/hook?key=existing&access_token=token", Secret: "SEC-test", Format: format}, p, time.UnixMilli(1700000000123))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err = json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			if req.URL.Query().Get("key") != "existing" || req.URL.Query().Get("access_token") != "token" {
				t.Fatal("URL credentials lost")
			}
			switch format {
			case "generic":
				var decoded notificationPayload
				json.Unmarshal(raw, &decoded)
				if decoded != p || !strings.HasPrefix(req.Header.Get("X-MonitorGo-Signature"), "sha256=") {
					t.Fatal("generic contract changed")
				}
			case "dingtalk":
				if body["msgtype"] != "markdown" || req.URL.Query().Get("timestamp") != "1700000000123" || req.URL.Query().Get("sign") != "NpzoKTncQ7MSxbFibBrce+1ub+vlCBaXVYQzsx1KJ5Y=" {
					t.Fatalf("DingTalk signature/body: %s %s", req.URL, raw)
				}
			case "feishu":
				if body["msg_type"] != "text" || body["timestamp"] != "1700000000" || body["sign"] != "wVkXGs7tnGGdFP0ROb5TA58qvnldaoxe43r/OZmowJ8=" {
					t.Fatalf("Feishu signature/body: %s", raw)
				}
			case "wecom":
				if body["msgtype"] != "markdown" || body["sign"] != nil || req.URL.Query().Get("sign") != "" {
					t.Fatal("unexpected WeCom signing")
				}
			}
			if format != "generic" {
				if req.Header.Get("X-MonitorGo-Signature") != "" {
					t.Fatal("generic signature leaked into platform format")
				}
				for _, text := range []string{p.Title, p.Description, p.Resource, p.Severity, "42", "2023-11-14T22:13:20Z", "Alert"} {
					if !strings.Contains(string(raw), text) {
						t.Errorf("message missing %q", text)
					}
				}
			}
		})
	}
	p.Title = strings.Repeat("标题", 1000)
	p.Description = strings.Repeat("诊断", 10000)
	p.Resource = strings.Repeat("服务", 1000)
	message := chatMessage(p)
	if len(message) > 4096 || !utf8.ValidString(message) || !strings.Contains(message, "Delivery: 42") || !strings.Contains(message, "…") {
		t.Fatal("long message invalid")
	}
}

func TestChatWebhookHTTPResponses(t *testing.T) {
	for _, tc := range []struct {
		format, body string
		ok           bool
	}{
		{"dingtalk", `{"errcode":0}`, true}, {"dingtalk", `{"errcode":310000,"errmsg":"redacted"}`, false},
		{"feishu", `{"code":0}`, true}, {"feishu", `{"code":19021}`, false},
		{"wecom", `{"errcode":0}`, true}, {"wecom", `{"errcode":93000}`, false},
		{"wecom", `{}`, false}, {"wecom", `{"errcode":null}`, false}, {"feishu", `{"code":"0"}`, false},
		{"dingtalk", `not JSON`, false}, {"generic", `not JSON`, true},
	} {
		t.Run(tc.format+tc.body, func(t *testing.T) {
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				io.WriteString(w, tc.body)
			}))
			defer receiver.Close()
			detail, err := sendWebhook(context.Background(), channelConfig{Format: tc.format, URL: receiver.URL}, notificationPayload{Event: "test"})
			if (err == nil) != tc.ok {
				t.Fatalf("result %q %v", detail, err)
			}
			if strings.Contains(detail, "redacted") {
				t.Fatal("platform response was reflected")
			}
		})
	}
}

func TestChatWebhookBusinessErrorsRetry(t *testing.T) {
	s, _, c := fixture(t)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"errcode":310000}`) }))
	defer receiver.Close()
	channel := decodeValue[map[string]string](t, c.call("POST", "/notification-channels", channelInput{Name: "DingTalk test", Kind: "webhook", Enabled: true, Config: channelConfig{Format: "dingtalk", URL: receiver.URL}}, 200))
	delivery := decodeValue[map[string]string](t, c.call("POST", "/notification-channels/"+channel["id"]+"/tests", map[string]int{"version": 1}, 200))
	var id int64
	if err := s.Store.DB.QueryRow("SELECT id FROM notification_deliveries WHERE id=?", delivery["deliveryId"]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i, delay := range []int64{15000, 60000, 300000, 0} {
		start := now()
		runDelivery(t, s, id)
		var state string
		var attempts int
		var next int64
		if err := s.Store.DB.QueryRow("SELECT state,attempts,next_attempt FROM notification_deliveries WHERE id=?", id).Scan(&state, &attempts, &next); err != nil {
			t.Fatal(err)
		}
		want := "queued"
		if i == 3 {
			want = "failed"
		}
		if state != want || attempts != i+1 || next < start+delay {
			t.Fatalf("attempt %d: %s %d %d", i, state, attempts, next)
		}
	}
}
