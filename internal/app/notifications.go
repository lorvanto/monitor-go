package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type notificationPayload struct {
	DeliveryID  string `json:"deliveryId"`
	Event       string `json:"event"`
	At          int64  `json:"at"`
	AlertID     string `json:"alertId,omitempty"`
	Resource    string `json:"resource,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func queueAlertNotifications(tx *sql.Tx, id int64, event, description string) error {
	p := notificationPayload{Event: event, At: now(), AlertID: idText(id)}
	if e := tx.QueryRow("SELECT resource,severity,title,description FROM alerts WHERE id=?", id).Scan(&p.Resource, &p.Severity, &p.Title, &p.Description); e != nil {
		return e
	}
	if event == "resolved" {
		p.Description = description
	}
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT OR IGNORE INTO notification_deliveries(channel_id,alert_id,event,event_key,payload,next_attempt,created_at,channel_version) SELECT id,?,?,?,?,?,?,version FROM notification_channels WHERE enabled=1 AND deleted_at IS NULL`, id, event, fmt.Sprintf("alert:%d:%s", id, event), string(raw), now()+5000, now())
	return e
}

func (s *Server) notificationLoop(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	slots := make(chan struct{}, 2)
	var work sync.WaitGroup
	defer work.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for len(slots) < cap(slots) {
				var id int64
				e := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM notification_deliveries WHERE state='queued' AND next_attempt<=? ORDER BY id LIMIT 1", now()).Scan(&id)
				if e != nil {
					break
				}
				res, e := s.Store.DB.ExecContext(ctx, "UPDATE notification_deliveries SET state='sending' WHERE id=? AND state='queued'", id)
				if e != nil {
					break
				}
				n, _ := res.RowsAffected()
				if n == 0 {
					continue
				}
				slots <- struct{}{}
				work.Add(1)
				go func() {
					defer work.Done()
					defer func() { <-slots }()
					if e := s.deliver(ctx, id); e != nil {
						slog.Error("通知任务入库失败", "delivery", id)
						s.Store.DB.Exec("UPDATE notification_deliveries SET state='queued' WHERE id=? AND state='sending'", id)
					}
				}()
			}
		}
	}
}
func (s *Server) deliver(ctx context.Context, id int64) error {
	var encrypted, kind, raw, event string
	var attempts int
	var suppressed string
	e := s.Store.write(ctx, func(tx *sql.Tx) error {
		var enabled bool
		var deleted, alertID sql.NullInt64
		var version, taskVersion int
		e := tx.QueryRow(`SELECT c.config,c.kind,c.enabled,c.deleted_at,c.version,d.channel_version,d.payload,d.event,d.alert_id,d.attempts FROM notification_deliveries d JOIN notification_channels c ON c.id=d.channel_id WHERE d.id=? AND d.state='sending'`, id).Scan(&encrypted, &kind, &enabled, &deleted, &version, &taskVersion, &raw, &event, &alertID, &attempts)
		if e != nil {
			return e
		}
		if !enabled || deleted.Valid || version != taskVersion {
			suppressed = "渠道停用、归档或配置已变更"
		}
		if alertID.Valid {
			var state string
			var until int64
			if e = tx.QueryRow("SELECT state,silence_until FROM alerts WHERE id=?", alertID.Int64).Scan(&state, &until); e != nil {
				return e
			}
			if until > now() {
				suppressed = "事件处于静默期，通知已跳过"
			} else if event == "opened" && state == "resolved" {
				suppressed = "故障已恢复，取消过时通知"
			}
			var revoked int
			if e = tx.QueryRow("SELECT count(*) FROM alerts a JOIN hosts h ON a.resource_kind='host' AND a.resource_id=h.id WHERE a.id=? AND h.source='agent' AND h.agent_enabled=0", alertID.Int64).Scan(&revoked); e != nil {
				return e
			}
			if revoked > 0 {
				suppressed = "Agent 接入已撤销"
			}
		}
		if suppressed != "" {
			_, e = tx.Exec("UPDATE notification_deliveries SET state='suppressed',detail=?,finished_at=? WHERE id=?", suppressed, now(), id)
			return e
		}
		_, e = tx.Exec("UPDATE notification_deliveries SET attempts=attempts+1 WHERE id=?", id)
		return e
	})
	if e != nil || suppressed != "" {
		return e
	}
	var cfg channelConfig
	var p notificationPayload
	config, e := s.Store.decrypt(encrypted)
	if e == nil {
		e = json.Unmarshal([]byte(config), &cfg)
	}
	if e == nil {
		e = json.Unmarshal([]byte(raw), &p)
	}
	detail := "通知配置无法解密或解析"
	if e == nil {
		p.DeliveryID = idText(id)
		sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		if kind == "webhook" {
			detail, e = sendWebhook(sendCtx, cfg, p)
		} else {
			detail, e = sendSMTP(sendCtx, cfg, p, nil)
		}
		cancel()
	}
	// On shutdown, retain pending work. A stable delivery ID lets receivers
	// deduplicate the unavoidable response-loss window (at-least-once delivery).
	if ctx.Err() != nil {
		_, err := s.Store.DB.Exec("UPDATE notification_deliveries SET state='queued' WHERE id=?", id)
		return err
	}
	attempts++
	state := "sent"
	var finished any = now()
	next := now()
	if e != nil {
		state = "failed"
		if attempts < 4 {
			state = "queued"
			finished = nil
			next = now() + []int64{15000, 60000, 300000}[attempts-1]
		}
	}
	_, e = s.Store.DB.ExecContext(ctx, "UPDATE notification_deliveries SET state=?,detail=?,next_attempt=?,finished_at=? WHERE id=?", state, detail, next, finished, id)
	return e
}
func sendWebhook(ctx context.Context, c channelConfig, p notificationPayload) (string, error) {
	body, e := json.Marshal(p)
	if e != nil {
		return "通知编码失败", e
	}
	tr := &http.Transport{DialContext: safeDial, TLSHandshakeTimeout: 5 * time.Second, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("通知不跟随重定向") }}
	req, e := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(body))
	if e != nil {
		return "Webhook 地址无效", e
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MonitorGo/0.2.0")
	req.Header.Set("X-MonitorGo-Delivery-ID", p.DeliveryID)
	req.Header.Set("X-MonitorGo-Timestamp", timestamp)
	if c.Secret != "" {
		mac := hmac.New(sha256.New, []byte(c.Secret))
		mac.Write([]byte(timestamp + "."))
		mac.Write(body)
		req.Header.Set("X-MonitorGo-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	res, e := client.Do(req)
	if e != nil {
		return "Webhook 连接失败，请检查网络、地址和 TLS 证书", e
	}
	defer res.Body.Close()
	io.CopyN(io.Discard, res.Body, 4096)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Sprintf("Webhook 返回 HTTP %d", res.StatusCode), errors.New("webhook rejected")
	}
	return fmt.Sprintf("Webhook 已接收 · HTTP %d", res.StatusCode), nil
}
func notificationMessage(c channelConfig, p notificationPayload) []byte {
	label := map[string]string{"opened": "故障告警", "resolved": "恢复通知", "test": "渠道测试"}[p.Event]
	subject := mime.BEncoding.Encode("UTF-8", "[Monitor Go · "+label+"] "+p.Title)
	body := fmt.Sprintf("%s\r\n\r\n资源：%s\r\n级别：%s\r\n内容：%s\r\n时间：%s\r\n事件：%s\r\n投递编号：%s\r\n", p.Title, p.Resource, p.Severity, p.Description, time.UnixMilli(p.At).UTC().Format(time.RFC3339), p.AlertID, p.DeliveryID)
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <monitor-go-%s-%d@monitor.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n", c.From, strings.Join(c.To, ", "), subject, time.Now().UTC().Format(time.RFC1123Z), p.DeliveryID, p.At)
	return []byte(headers + body)
}
func sendSMTP(ctx context.Context, c channelConfig, p notificationPayload, trust *tls.Config) (string, error) {
	conn, e := safeDial(ctx, "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if e != nil {
		return "SMTP 连接失败", e
	}
	defer conn.Close()
	rawConn := conn
	stopClose := context.AfterFunc(ctx, func() { rawConn.Close() })
	defer stopClose()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(12 * time.Second)
	}
	conn.SetDeadline(deadline)
	tc := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if trust != nil {
		tc = trust.Clone()
		tc.ServerName = c.Host
		tc.MinVersion = tls.VersionTLS12
	}
	if c.Security == "tls" {
		secure := tls.Client(conn, tc)
		if e = secure.HandshakeContext(ctx); e != nil {
			return "SMTP TLS 证书验证或握手失败", e
		}
		conn = secure
	}
	client, e := smtp.NewClient(conn, c.Host)
	if e != nil {
		return "SMTP 服务握手失败", e
	}
	defer client.Close()
	if c.Security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return "SMTP 服务未提供 STARTTLS，已拒绝明文发送", errors.New("STARTTLS required")
		}
		if e = client.StartTLS(tc); e != nil {
			return "SMTP STARTTLS 验证失败", e
		}
	}
	if c.Username != "" {
		if e = client.Auth(smtp.PlainAuth("", c.Username, c.Secret, c.Host)); e != nil {
			return "SMTP 认证失败，请检查账号和授权码", e
		}
	}
	if e = client.Mail(c.From); e != nil {
		return "SMTP 拒绝发件地址", e
	}
	for _, to := range c.To {
		if e = client.Rcpt(to); e != nil {
			return "SMTP 拒绝收件地址", e
		}
	}
	writer, e := client.Data()
	if e != nil {
		return "SMTP 无法接收邮件内容", e
	}
	if _, e = writer.Write(notificationMessage(c, p)); e != nil {
		return "SMTP 邮件写入失败", e
	}
	if e = writer.Close(); e != nil {
		return "SMTP 未确认接收邮件", e
	}
	// A successful DATA response is the acceptance boundary; a QUIT failure
	// must not cause duplicate delivery of an already-accepted message.
	client.Quit()
	return "SMTP 服务已接收邮件", nil
}
