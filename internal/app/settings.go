package app

import (
	"database/sql"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type requestIDKey struct{}
type streamed struct{}

func auditRequest(tx *sql.Tx, r *http.Request, actor, action, resource string) error {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	_, e := tx.Exec("INSERT INTO audit_logs(at,actor,action,resource,request_id,method,path) VALUES(?,?,?,?,?,?,?)", now(), actor, action, resource, id, r.Method, r.URL.Path)
	return e
}

type Settings struct {
	RawRetentionDays     int    `json:"rawRetentionDays"`
	SummaryRetentionDays int    `json:"summaryRetentionDays"`
	HostCPUPercent       int    `json:"hostCPUPercent"`
	HostMemoryPercent    int    `json:"hostMemoryPercent"`
	HostDiskPercent      int    `json:"hostDiskPercent"`
	HostAlertMinutes     int    `json:"hostAlertMinutes"`
	InstanceName         string `json:"instanceName"`
	UseDefaultName       bool   `json:"useDefaultName"`
	UploadLimitMB        int64  `json:"uploadLimitMB"`
	StorageLimitMB       int64  `json:"storageLimitMB"`
	DefaultFolder        string `json:"defaultFolder"`
	AuditDays            int    `json:"auditDays"`
	Version              int    `json:"version"`
}
type rowQuery interface{ QueryRow(string, ...any) *sql.Row }

// Empty names explicitly select the localized default. Version 1 also identifies
// the untouched legacy default; saved custom names, even identical text, stay literal.
func defaultInstanceName(name string, version int) bool {
	return name == "" || (version == 1 && name == "我的监控中心")
}

func readSettings(q rowQuery) (Settings, error) {
	var v Settings
	e := q.QueryRow("SELECT instance_name,upload_limit_mb,storage_limit_mb,CAST(default_folder AS TEXT),audit_days,version,host_cpu_percent,host_memory_percent,host_disk_percent,host_alert_minutes,raw_retention_days,summary_retention_days FROM system_settings WHERE id=1").Scan(&v.InstanceName, &v.UploadLimitMB, &v.StorageLimitMB, &v.DefaultFolder, &v.AuditDays, &v.Version, &v.HostCPUPercent, &v.HostMemoryPercent, &v.HostDiskPercent, &v.HostAlertMinutes, &v.RawRetentionDays, &v.SummaryRetentionDays)
	v.UseDefaultName = defaultInstanceName(v.InstanceName, v.Version)
	if v.UseDefaultName {
		v.InstanceName = ""
	}
	return v, e
}
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return readSettings(s.Store.DB)
}
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in Settings
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	in.InstanceName = strings.TrimSpace(in.InstanceName)
	if in.UseDefaultName {
		in.InstanceName = ""
	}
	if !in.UseDefaultName && (in.InstanceName == "" || len(in.InstanceName) > 120) {
		return nil, bad("实例名称为 1–120 字节")
	}
	if in.UploadLimitMB < 1 || in.UploadLimitMB > 1024 || in.StorageLimitMB < 10 || in.StorageLimitMB > 1048576 || in.AuditDays < 7 || in.AuditDays > 365 {
		return nil, bad("上传上限为 1–1024 MB，存储配额为 10–1048576 MB，审计保留 7–365 天")
	}
	if in.HostCPUPercent < 0 || in.HostCPUPercent > 100 || in.HostMemoryPercent < 0 || in.HostMemoryPercent > 100 || in.HostDiskPercent < 0 || in.HostDiskPercent > 100 || in.HostAlertMinutes < 1 || in.HostAlertMinutes > 60 {
		return nil, bad("主机阈值为 0–100%，持续时长为 1–60 分钟")
	}
	if in.RawRetentionDays < 1 || in.RawRetentionDays > 30 || in.SummaryRetentionDays < 90 || in.SummaryRetentionDays > 730 {
		return nil, bad("原始指标保留 1–30 天，汇总保留 90–730 天")
	}
	e := s.Store.write(r.Context(), func(tx *sql.Tx) error {
		old, e := readSettings(tx)
		if e != nil {
			return e
		}
		if old.Version != in.Version {
			return conflict()
		}
		var count int
		if e = tx.QueryRow("SELECT count(*) FROM files WHERE id=? AND kind='folder' AND deleted_at IS NULL AND purged_at IS NULL", in.DefaultFolder).Scan(&count); e != nil {
			return e
		}
		if count != 1 {
			return bad("默认上传目录不存在或已在回收站")
		}
		var used int64
		if e = tx.QueryRow("SELECT coalesce(sum(size),0) FROM files WHERE object_key<>''").Scan(&used); e != nil {
			return e
		}
		if used > in.StorageLimitMB*1048576 {
			return bad("存储配额不能小于当前占用（含回收站）")
		}
		_, e = tx.Exec("UPDATE system_settings SET instance_name=?,upload_limit_mb=?,storage_limit_mb=?,default_folder=?,audit_days=?,host_cpu_percent=?,host_memory_percent=?,host_disk_percent=?,host_alert_minutes=?,raw_retention_days=?,summary_retention_days=?,version=version+1 WHERE id=1", in.InstanceName, in.UploadLimitMB, in.StorageLimitMB, in.DefaultFolder, in.AuditDays, in.HostCPUPercent, in.HostMemoryPercent, in.HostDiskPercent, in.HostAlertMinutes, in.RawRetentionDays, in.SummaryRetentionDays)
		if e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "修改系统设置", in.InstanceName)
	})
	return true, e
}
func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var schema, files, folders, pending int64
	var size int64
	e := s.Store.DB.QueryRowContext(r.Context(), "SELECT (SELECT max(version) FROM schema_migrations),(SELECT count(*) FROM files WHERE kind='file' AND purged_at IS NULL),(SELECT count(*) FROM files WHERE kind='folder' AND deleted_at IS NULL),(SELECT coalesce(sum(size),0) FROM files WHERE object_key<>''),(SELECT count(*) FROM files WHERE purged_at IS NOT NULL AND object_key<>'')").Scan(&schema, &files, &folders, &size, &pending)
	if e != nil {
		return nil, e
	}
	var dbSize int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, e := os.Stat(filepath.Join(s.Store.Dir, "monitor.db"+suffix)); e == nil {
			dbSize += info.Size()
		}
	}
	return map[string]any{"version": buildinfo.Version, "goVersion": runtime.Version(), "platform": runtime.GOOS + " / " + runtime.GOARCH, "database": "SQLite", "schema": schema, "databaseSize": dbSize, "files": files, "folders": folders, "storageUsed": size, "pendingGC": pending}, nil
}
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	if before == 0 {
		before = 1 << 62
	}
	search := strings.TrimSpace(q.Get("q"))
	if len(search) > 120 {
		return nil, bad("搜索词过长")
	}
	from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
	to, _ := strconv.ParseInt(q.Get("to"), 10, 64)
	if to == 0 {
		to = 1 << 62
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT id,at,actor,action,resource,request_id,method,path FROM audit_logs WHERE id<? AND at>=? AND at<=? AND (?='' OR instr(actor||' '||action||' '||resource||' '||request_id,?)>0) ORDER BY id DESC LIMIT 51", before, from, to, search, search)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	type entry struct {
		ID        string `json:"id"`
		At        int64  `json:"at"`
		Actor     string `json:"actor"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		RequestID string `json:"requestId"`
		Method    string `json:"method"`
		Path      string `json:"path"`
	}
	entries := []entry{}
	for rows.Next() {
		var v entry
		if e = rows.Scan(&v.ID, &v.At, &v.Actor, &v.Action, &v.Resource, &v.RequestID, &v.Method, &v.Path); e != nil {
			return nil, e
		}
		entries = append(entries, v)
	}
	more := len(entries) > 50
	if more {
		entries = entries[:50]
	}
	return map[string]any{"items": entries, "hasMore": more}, rows.Err()
}
