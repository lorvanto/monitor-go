package app

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const monitorColumns = `id,name,type,target,group_name,interval_seconds,timeout_seconds,expected_status,failure_threshold,ssl_days,username,secret,tls,enabled,state,latency,detail,checked_at,next_check,version`

func scanMonitor(row interface{ Scan(...any) error }) (Monitor, error) {
	m := Monitor{History: []CheckResult{}}
	e := row.Scan(&m.ID, &m.Name, &m.Type, &m.Target, &m.Group, &m.Interval, &m.Timeout, &m.Expected, &m.Threshold, &m.SSLDays, &m.Username, &m.Secret, &m.TLS, &m.Enabled, &m.State, &m.Latency, &m.Detail, &m.Checked, &m.NextCheck, &m.Version)
	m.HasSecret = m.Secret != ""
	if !m.Enabled {
		m.State = "paused"
	}
	return m, e
}

const monitorUptimeQuery = `SELECT m.id,count(*) FILTER (WHERE r.state IN ('healthy','warning')),count(*)
FROM monitors m JOIN check_results r ON r.monitor_id=m.id AND r.at>=? WHERE m.deleted_at IS NULL GROUP BY m.id`
const monitorHistoryQuery = `SELECT monitor_id,at,state,latency,detail FROM (
 SELECT r.monitor_id,r.at,r.state,r.latency,r.detail,
 ROW_NUMBER() OVER (PARTITION BY r.monitor_id ORDER BY r.at DESC,r.id DESC) AS rank
 FROM monitors m JOIN check_results r ON r.id IN (
  SELECT id FROM check_results WHERE monitor_id=m.id ORDER BY at DESC,id DESC LIMIT 32
 ) WHERE m.deleted_at IS NULL
) WHERE rank<=32 ORDER BY monitor_id,rank`

func (s *Server) monitors(ctx context.Context) ([]Monitor, error) {
	rows, e := s.Store.DB.QueryContext(ctx, "SELECT "+monitorColumns+" FROM monitors WHERE deleted_at IS NULL ORDER BY id DESC")
	if e != nil {
		return nil, e
	}
	out := []Monitor{}
	for rows.Next() {
		m, e := scanMonitor(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}

	byID := make(map[int64]*Monitor, len(out))
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	rows, e = s.Store.DB.QueryContext(ctx, monitorUptimeQuery, now()-86400000)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var id int64
		var good, total int
		if e = rows.Scan(&id, &good, &total); e != nil {
			rows.Close()
			return nil, e
		}
		if m := byID[id]; m != nil && total > 0 {
			value := float64(good) * 100 / float64(total)
			m.Uptime = &value
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	rows, e = s.Store.DB.QueryContext(ctx, monitorHistoryQuery)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var id int64
		var result CheckResult
		if e = rows.Scan(&id, &result.At, &result.State, &result.Latency, &result.Detail); e != nil {
			rows.Close()
			return nil, e
		}
		if m := byID[id]; m != nil {
			m.History = append(m.History, result)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}

	return out, nil
}
func (s *Server) listMonitors(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return s.monitors(r.Context())
}

type monitorInput struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Target    string `json:"target"`
	Group     string `json:"group"`
	Interval  int    `json:"interval"`
	Timeout   int    `json:"timeout"`
	Expected  int    `json:"expected"`
	Threshold int    `json:"threshold"`
	SSLDays   int    `json:"sslDays"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	TLS       bool   `json:"tls"`
}

func (i *monitorInput) validate() error {
	i.Name = strings.TrimSpace(i.Name)
	i.Target = strings.TrimSpace(i.Target)
	i.Group = strings.TrimSpace(i.Group)
	if len(i.Name) == 0 || len(i.Name) > 100 || len(i.Target) == 0 || len(i.Target) > 2048 || len(i.Group) > 80 || len(i.Username) > 128 || len(i.Password) > 1024 {
		return bad("检测名称、地址或分组长度不正确")
	}
	if i.Group == "" {
		i.Group = "默认分组"
	}
	if i.Interval < 15 || i.Interval > 86400 || i.Timeout < 1 || i.Timeout > 30 || i.Timeout >= i.Interval || i.Threshold < 1 || i.Threshold > 10 || i.SSLDays < 1 || i.SSLDays > 90 {
		return bad("周期 15–86400 秒；超时 1–30 秒且短于周期；失败阈值 1–10 次；证书提醒 1–90 天")
	}
	if i.Expected < 100 || i.Expected > 599 {
		return bad("预期状态码无效")
	}
	switch i.Type {
	case "HTTP":
		u, e := url.Parse(i.Target)
		if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
			return bad("HTTP 目标需要有效的 http(s) 地址，不允许 URL 中包含登录凭据")
		}
	case "TCP", "MySQL", "Redis", "SSL":
		host, port, e := net.SplitHostPort(i.Target)
		if e != nil || host == "" {
			return bad("目标需要域名或 IP 加端口，例如 example.com:443")
		}
		p, e := strconv.Atoi(port)
		if e != nil || p < 1 || p > 65535 {
			return bad("端口范围为 1–65535")
		}
		if i.Type == "MySQL" && i.Username == "" {
			return bad("MySQL 检测需要用户名")
		}
	default:
		return bad("不支持的检测类型")
	}
	return nil
}
func (s *Server) createMonitor(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in monitorInput
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	if e := in.validate(); e != nil {
		return nil, e
	}
	secret, e := s.Store.encrypt(in.Password)
	if e != nil {
		return nil, e
	}
	var id int64
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var n int
		if e := tx.QueryRow("SELECT count(*) FROM monitors WHERE deleted_at IS NULL").Scan(&n); e != nil {
			return e
		}
		if n >= 500 {
			return bad("当前版本单套安装最多配置 500 个检测目标")
		}
		res, e := tx.Exec(`INSERT INTO monitors(name,type,target,group_name,interval_seconds,timeout_seconds,expected_status,failure_threshold,ssl_days,username,secret,tls) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, in.Name, in.Type, in.Target, in.Group, in.Interval, in.Timeout, in.Expected, in.Threshold, in.SSLDays, in.Username, secret, in.TLS)
		if e != nil {
			return e
		}
		id, _ = res.LastInsertId()
		return auditRequest(tx, r, u.Name, "新增检测", in.Name)
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) updateMonitor(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int  `json:"version"`
		Enabled bool `json:"enabled"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		res, e := tx.Exec("UPDATE monitors SET enabled=?,state=CASE WHEN ? THEN 'unknown' ELSE state END,version=version+1,next_check=0,failures=0 WHERE id=? AND version=? AND deleted_at IS NULL", in.Enabled, in.Enabled, id, in.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict()
		}
		if _, e = tx.Exec("UPDATE jobs SET state='cancelled',finished_at=? WHERE monitor_id=? AND state='queued'", now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "切换检测启停", idText(id))
	})
	return true, e
}
func (s *Server) deleteMonitor(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
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
		res, e := tx.Exec("UPDATE monitors SET deleted_at=?,enabled=0,version=version+1 WHERE id=? AND version=? AND deleted_at IS NULL", now(), id, in.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict()
		}
		if _, e = tx.Exec("UPDATE jobs SET state='cancelled',finished_at=? WHERE monitor_id=? AND state='queued'", now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "归档检测目标", idText(id))
	})
	return true, e
}
func (s *Server) manualCheck(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	job, e := s.enqueue(r.Context(), id)
	return map[string]string{"jobId": idText(job)}, e
}
func (s *Server) enqueue(ctx context.Context, id int64) (int64, error) {
	var job int64
	e := s.Store.write(ctx, func(tx *sql.Tx) error {
		var enabled bool
		if e := tx.QueryRow("SELECT enabled FROM monitors WHERE id=? AND deleted_at IS NULL", id).Scan(&enabled); e == sql.ErrNoRows {
			return missing()
		} else if e != nil {
			return e
		}
		if !enabled {
			return bad("检测已暂停")
		}
		var n int
		if e := tx.QueryRow("SELECT count(*) FROM jobs WHERE state IN ('queued','running')").Scan(&n); e != nil {
			return e
		}
		if n >= 256 {
			return &apiError{429, "检测队列已满，请稍后重试"}
		}
		if _, e := tx.Exec("INSERT OR IGNORE INTO jobs(monitor_id,created_at) VALUES(?,?)", id, now()); e != nil {
			return e
		}
		if e := tx.QueryRow("SELECT id FROM jobs WHERE monitor_id=? AND state IN ('queued','running')", id).Scan(&job); e != nil {
			return e
		}
		_, e := tx.Exec("UPDATE monitors SET next_check=?+interval_seconds*1000 WHERE id=?", now(), id)
		return e
	})
	return job, e
}
func rangeHours(r *http.Request) int64 {
	switch r.URL.Query().Get("hours") {
	case "1":
		return 1
	case "168":
		return 168
	default:
		return 24
	}
}
func since(r *http.Request) int64 {
	return time.Now().Add(-time.Duration(rangeHours(r)) * time.Hour).UnixMilli()
}
