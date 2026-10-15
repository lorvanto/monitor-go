package app

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
)

type channelConfig struct {
	URL      string   `json:"url"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Security string   `json:"security"`
	Username string   `json:"username"`
	Secret   string   `json:"secret"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}
type notificationChannel struct {
	ID        int64         `json:"id,string"`
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Enabled   bool          `json:"enabled"`
	Version   int           `json:"version"`
	Config    channelConfig `json:"config"`
	HasSecret bool          `json:"hasSecret"`
}
type channelInput struct {
	Name        string        `json:"name"`
	Kind        string        `json:"kind"`
	Enabled     bool          `json:"enabled"`
	Version     int           `json:"version"`
	Config      channelConfig `json:"config"`
	ClearSecret bool          `json:"clearSecret"`
}

func validateChannel(in *channelInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 {
		return bad("通知渠道名称长度为 1–120 字节")
	}
	c := &in.Config
	if len(c.Secret) > 4096 {
		return bad("通知凭据过长")
	}
	if in.Kind == "webhook" {
		u, e := url.Parse(c.URL)
		if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(c.URL) > 2048 {
			return bad("Webhook 地址不正确")
		}
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
			return bad("Webhook 必须使用 HTTPS，回环地址允许 HTTP 联调")
		}
		if u.Port() != "" {
			p, e := strconv.Atoi(u.Port())
			if e != nil || p < 1 || p > 65535 {
				return bad("Webhook 端口不正确")
			}
		}
		*c = channelConfig{URL: c.URL, Secret: c.Secret, To: []string{}}
	} else if in.Kind == "smtp" {
		c.Host = strings.TrimSpace(c.Host)
		if c.Host == "" || len(c.Host) > 253 || strings.ContainsAny(c.Host, "/\r\n @") || c.Port < 1 || c.Port > 65535 {
			return bad("SMTP 地址或端口不正确")
		}
		if c.Security != "tls" && c.Security != "starttls" {
			return bad("SMTP 需使用 TLS 或 STARTTLS")
		}
		if len(c.Username) > 254 || strings.ContainsAny(c.Username, "\r\n") {
			return bad("SMTP 用户名不正确")
		}
		validMail := func(s string) bool {
			a, e := mail.ParseAddress(s)
			return e == nil && a.Address == s && !strings.ContainsAny(s, "\r\n") && len(s) <= 254
		}
		if !validMail(c.From) || len(c.To) < 1 || len(c.To) > 20 {
			return bad("请填写发件邮箱和 1–20 个收件邮箱")
		}
		for _, to := range c.To {
			if !validMail(to) {
				return bad("收件邮箱格式不正确")
			}
		}
		if c.Username != "" && c.Secret == "" {
			return bad("SMTP 登录需要密码或授权码")
		}
		c.URL = ""
	} else {
		return bad("通知类型仅支持 Webhook 和 SMTP 邮件")
	}
	return nil
}
func (s *Server) listChannels(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := []notificationChannel{}
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT id,name,kind,enabled,version,config FROM notification_channels WHERE deleted_at IS NULL ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var c notificationChannel
		var encrypted string
		if e = rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Enabled, &c.Version, &encrypted); e != nil {
			return nil, e
		}
		raw, e := s.Store.decrypt(encrypted)
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &c.Config); e != nil {
			return nil, e
		}
		c.HasSecret = c.Config.Secret != ""
		c.Config.Secret = ""
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Server) saveChannel(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in channelInput
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	var id int64
	var e error
	if r.Method == "PATCH" {
		id, e = pathID(r)
		if e != nil {
			return nil, e
		}
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		if id > 0 {
			var old, kind string
			var version int
			if e := tx.QueryRow("SELECT config,kind,version FROM notification_channels WHERE id=? AND deleted_at IS NULL", id).Scan(&old, &kind, &version); e == sql.ErrNoRows {
				return missing()
			} else if e != nil {
				return e
			}
			if version != in.Version {
				return conflict()
			}
			if kind != in.Kind {
				return bad("渠道类型不能更改，请新增渠道")
			}
			raw, e := s.Store.decrypt(old)
			if e != nil {
				return e
			}
			var c channelConfig
			if e = json.Unmarshal([]byte(raw), &c); e != nil {
				return e
			}
			if in.ClearSecret {
				in.Config.Secret = ""
			} else if in.Config.Secret == "" {
				in.Config.Secret = c.Secret
			}
		} else {
			var count int
			if e := tx.QueryRow("SELECT count(*) FROM notification_channels WHERE deleted_at IS NULL").Scan(&count); e != nil {
				return e
			}
			if count >= 20 {
				return bad("最多配置 20 个通知渠道")
			}
		}
		if e := validateChannel(&in); e != nil {
			return e
		}
		raw, e := json.Marshal(in.Config)
		if e != nil {
			return e
		}
		encrypted, e := s.Store.encrypt(string(raw))
		if e != nil {
			return e
		}
		if id == 0 {
			res, e := tx.Exec("INSERT INTO notification_channels(name,kind,config,enabled) VALUES(?,?,?,?)", in.Name, in.Kind, encrypted, in.Enabled)
			if e != nil {
				return e
			}
			id, e = res.LastInsertId()
			if e != nil {
				return e
			}
		} else {
			if _, e = tx.Exec("UPDATE notification_channels SET name=?,config=?,enabled=?,version=version+1 WHERE id=?", in.Name, encrypted, in.Enabled, id); e != nil {
				return e
			}
			if _, e = tx.Exec("UPDATE notification_deliveries SET state='suppressed',finished_at=?,detail='渠道配置已变更，旧任务已取消' WHERE channel_id=? AND state='queued'", now(), id); e != nil {
				return e
			}
		}
		return auditRequest(tx, r, u.Name, "保存通知渠道", idText(id))
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		res, e := tx.Exec("UPDATE notification_channels SET deleted_at=?,enabled=0,version=version+1 WHERE id=? AND version=? AND deleted_at IS NULL", now(), id, in.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict()
		}
		if _, e = tx.Exec("UPDATE notification_deliveries SET state='suppressed',finished_at=?,detail='通知渠道已归档' WHERE channel_id=? AND state='queued'", now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "归档通知渠道", idText(id))
	})
	return true, e
}
func (s *Server) testChannel(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	var job int64
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var enabled bool
		var version int
		if e := tx.QueryRow("SELECT enabled,version FROM notification_channels WHERE id=? AND deleted_at IS NULL", id).Scan(&enabled, &version); e == sql.ErrNoRows {
			return missing()
		} else if e != nil {
			return e
		}
		if version != in.Version {
			return conflict()
		}
		if !enabled {
			return bad("请先启用渠道")
		}
		var count int
		if e := tx.QueryRow("SELECT count(*) FROM notification_deliveries WHERE channel_id=? AND event='test' AND created_at>?", id, now()-60000).Scan(&count); e != nil {
			return e
		}
		if count >= 3 {
			return &apiError{429, "每个渠道每分钟最多发送 3 条测试通知"}
		}
		payload, _ := json.Marshal(notificationPayload{Event: "test", At: now(), Title: "Monitor Go 测试通知", Description: "这是一条由管理员主动发送的通知，收到此消息表示渠道已连通。"})
		res, e := tx.Exec("INSERT INTO notification_deliveries(channel_id,event,event_key,payload,next_attempt,created_at,channel_version) VALUES(?,'test',?,?,?,?,?)", id, "test:"+randomToken(), string(payload), now(), now(), version)
		if e != nil {
			return e
		}
		job, e = res.LastInsertId()
		if e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "发送渠道测试通知", idText(id))
	})
	return map[string]string{"deliveryId": idText(job)}, e
}

type deliveryView struct {
	ID          int64  `json:"id,string"`
	Channel     string `json:"channel"`
	ChannelID   int64  `json:"channelId,string"`
	AlertID     *int64 `json:"alertId,omitempty,string"`
	Event       string `json:"event"`
	State       string `json:"state"`
	Attempts    int    `json:"attempts"`
	NextAttempt int64  `json:"nextAttempt"`
	CreatedAt   int64  `json:"createdAt"`
	FinishedAt  *int64 `json:"finishedAt"`
	Detail      string `json:"detail"`
}

func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := []deliveryView{}
	alertID := r.URL.Query().Get("alertId")
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT d.id,c.name,d.channel_id,d.alert_id,d.event,d.state,d.attempts,d.next_attempt,d.created_at,d.finished_at,d.detail FROM notification_deliveries d JOIN notification_channels c ON c.id=d.channel_id WHERE (?='' OR CAST(d.alert_id AS TEXT)=?) ORDER BY d.id DESC LIMIT 200`, alertID, alertID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var d deliveryView
		if e = rows.Scan(&d.ID, &d.Channel, &d.ChannelID, &d.AlertID, &d.Event, &d.State, &d.Attempts, &d.NextAttempt, &d.CreatedAt, &d.FinishedAt, &d.Detail); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Server) retryDelivery(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		res, e := tx.Exec(`UPDATE notification_deliveries SET state='queued',attempts=0,next_attempt=?,finished_at=NULL,detail='',channel_version=(SELECT version FROM notification_channels WHERE id=channel_id) WHERE id=? AND state='failed' AND channel_id IN(SELECT id FROM notification_channels WHERE enabled=1 AND deleted_at IS NULL)`, now(), id)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return bad("仅可重试已失败且渠道仍启用的通知")
		}
		return auditRequest(tx, r, u.Name, "重试告警通知", idText(id))
	})
	return true, e
}
