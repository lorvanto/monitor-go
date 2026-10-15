package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func webhookFormat(format string) string {
	if format == "" {
		return "generic"
	}
	return format
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit - len("…")
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "…"
}

// Fixed labels are English; user-authored alert text is preserved. Bound each
// field so WeCom's 4096-byte limit still leaves room for time and delivery ID.
func chatMessage(p notificationPayload) string {
	event := map[string]string{"opened": "Alert", "resolved": "Recovery", "test": "Test"}[p.Event]
	if event == "" {
		event = p.Event
	}
	return fmt.Sprintf("Monitor Go · %s\nTitle: %s\nResource: %s\nSeverity: %s\nDescription: %s\nTime: %s\nDelivery: %s", truncateUTF8(event, 40), truncateUTF8(p.Title, 500), truncateUTF8(p.Resource, 400), truncateUTF8(p.Severity, 30), truncateUTF8(p.Description, 2000), time.UnixMilli(p.At).UTC().Format(time.RFC3339), truncateUTF8(p.DeliveryID, 80))
}

func webhookRequest(ctx context.Context, c channelConfig, p notificationPayload, at time.Time) (*http.Request, error) {
	target := c.URL
	format := webhookFormat(c.Format)
	var payload any = p
	switch format {
	case "generic":
	case "dingtalk":
		payload = map[string]any{"msgtype": "markdown", "markdown": map[string]string{"title": truncateUTF8(p.Title, 500), "text": chatMessage(p)}}
		if c.Secret != "" {
			timestamp := strconv.FormatInt(at.UnixMilli(), 10)
			mac := hmac.New(sha256.New, []byte(c.Secret))
			mac.Write([]byte(timestamp + "\n" + c.Secret))
			parsed, err := url.Parse(target)
			if err != nil {
				return nil, err
			}
			q := parsed.Query()
			q.Set("timestamp", timestamp)
			q.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
			parsed.RawQuery = q.Encode()
			target = parsed.String()
		}
	case "feishu":
		body := map[string]any{"msg_type": "text", "content": map[string]string{"text": chatMessage(p)}}
		if c.Secret != "" {
			timestamp := strconv.FormatInt(at.Unix(), 10)
			mac := hmac.New(sha256.New, []byte(timestamp+"\n"+c.Secret))
			body["timestamp"] = timestamp
			body["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
		}
		payload = body
	case "wecom":
		payload = map[string]any{"msgtype": "markdown", "markdown": map[string]string{"content": chatMessage(p)}}
	default:
		return nil, errors.New("unsupported webhook format")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	req.Header.Set("X-MonitorGo-Delivery-ID", p.DeliveryID)
	if format == "generic" {
		timestamp := strconv.FormatInt(at.Unix(), 10)
		req.Header.Set("X-MonitorGo-Timestamp", timestamp)
		if c.Secret != "" {
			mac := hmac.New(sha256.New, []byte(c.Secret))
			mac.Write([]byte(timestamp + "."))
			mac.Write(body)
			req.Header.Set("X-MonitorGo-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
	}
	return req, nil
}

func webhookResponse(res *http.Response, format string) (string, error) {
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Sprintf("Webhook 返回 HTTP %d", res.StatusCode), errors.New("webhook rejected")
	}
	if webhookFormat(format) != "generic" {
		raw, err := io.ReadAll(io.LimitReader(res.Body, 65537))
		var result map[string]json.RawMessage
		if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &result) != nil {
			return "Webhook 响应格式不正确", errors.New("invalid webhook response")
		}
		key := "errcode"
		if format == "feishu" {
			key = "code"
		}
		var code int
		// Absence, null or a non-integer code must not be mistaken for success.
		value, exists := result[key]
		if !exists || strings.TrimSpace(string(value)) == "null" || json.Unmarshal(value, &code) != nil {
			return "Webhook 响应格式不正确", errors.New("missing webhook status")
		}
		if code != 0 {
			return fmt.Sprintf("Webhook 平台拒绝消息 · %d", code), errors.New("platform rejected webhook")
		}
	} else {
		io.CopyN(io.Discard, res.Body, 4096)
	}
	return fmt.Sprintf("Webhook 已接收 · HTTP %d", res.StatusCode), nil
}
