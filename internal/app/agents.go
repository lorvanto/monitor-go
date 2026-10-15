package app

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/hoststats"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// Enrollment secrets are short-lived and scoped to exactly one reserved host.
func (s *Server) createAgent(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Alias string `json:"alias"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	in.Alias = strings.TrimSpace(in.Alias)
	if in.Alias == "" || len(in.Alias) > 180 {
		return nil, bad("请输入 1–180 字节的主机名称")
	}
	token := randomToken()
	expires := now() + 15*60000
	var id int64
	inv, _ := json.Marshal(inventory{Source: "远程 Agent", IPs: []string{}})
	snap, _ := json.Marshal(snapshot{Disks: []diskInfo{}, Errors: []string{}})
	e := s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var count int
		if e := tx.QueryRow("SELECT count(*) FROM hosts WHERE source='agent'").Scan(&count); e != nil {
			return e
		}
		if count >= 1000 {
			return bad("当前最多登记 1000 台远程主机")
		}
		if e := tx.QueryRow("SELECT coalesce(max(id),1)+1 FROM hosts").Scan(&id); e != nil {
			return e
		}
		if id < 2 {
			id = 2
		}
		_, e := tx.Exec("INSERT INTO hosts(id,hostname,alias,inventory,snapshot,source,enrollment_hash,enrollment_expires) VALUES(?,?,?,?,?,'agent',?,?)", id, in.Alias, in.Alias, string(inv), string(snap), digest(token), expires)
		if e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "创建 Agent 接入", idText(id))
	})
	return map[string]any{"id": idText(id), "token": token, "expiresAt": expires}, e
}

func (s *Server) changeAgent(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int    `json:"version"`
		Action  string `json:"action"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	if in.Action != "revoke" && in.Action != "reenroll" {
		return nil, bad("不支持的 Agent 操作")
	}
	token := ""
	expires := int64(0)
	hash := ""
	if in.Action == "reenroll" {
		token = randomToken()
		hash = digest(token)
		expires = now() + 15*60000
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var source string
		var version int
		if e := tx.QueryRow("SELECT source,version FROM hosts WHERE id=?", id).Scan(&source, &version); e == sql.ErrNoRows {
			return missing()
		} else if e != nil {
			return e
		}
		if source != "agent" {
			return bad("本机采集无需 Agent 凭据")
		}
		if version != in.Version {
			return conflict()
		}
		_, e := tx.Exec("UPDATE hosts SET token_hash='',enrollment_hash=?,enrollment_expires=?,enrolled_at=0,agent_enabled=?,version=version+1,report_id='',reported_at=0 WHERE id=?", hash, expires, in.Action == "reenroll", id)
		if e != nil {
			return e
		}
		// Unsent notifications for an intentionally removed connection are cancelled.
		if in.Action == "revoke" {
			if _, e = tx.Exec("UPDATE notification_deliveries SET state='suppressed',detail='Agent 接入已撤销',finished_at=? WHERE state='queued' AND alert_id IN (SELECT id FROM alerts WHERE resource_kind='host' AND resource_id=?)", now(), id); e != nil {
				return e
			}
		}
		return auditRequest(tx, r, u.Name, map[bool]string{true: "重新签发 Agent 接入码", false: "撤销 Agent 接入"}[in.Action == "reenroll"], idText(id))
	})
	return map[string]any{"id": idText(id), "token": token, "expiresAt": expires}, e
}

func validToken(v string) bool { b, e := hex.DecodeString(v); return e == nil && len(b) == 32 }
func (s *Server) enrollAgent(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !s.allowedRequest(r, "agent:", 60) {
		return nil, &apiError{429, "接入请求过于频繁，请稍后重试"}
	}
	var in struct {
		Token      string `json:"token"`
		Credential string `json:"credential"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	if !validToken(in.Token) || !validToken(in.Credential) {
		return nil, &apiError{401, "接入码无效或已过期"}
	}
	var id int64
	e := s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var existing string
		e := tx.QueryRow("SELECT id,token_hash FROM hosts WHERE source='agent' AND agent_enabled=1 AND enrollment_hash=? AND enrollment_expires>?", digest(in.Token), now()).Scan(&id, &existing)
		if e == sql.ErrNoRows {
			return &apiError{401, "接入码无效或已过期"}
		}
		if e != nil {
			return e
		}
		credential := digest(in.Credential)
		// Repeating the same enrollment is safe after a lost response. A different
		// credential cannot consume an already-used enrollment code.
		if existing != "" {
			if existing == credential {
				return nil
			}
			return &apiError{409, "接入码已被使用"}
		}
		if _, e = tx.Exec("UPDATE hosts SET token_hash=?,enrolled_at=?,version=version+1 WHERE id=?", credential, now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, "Agent", "完成主机注册", idText(id))
	})
	return map[string]any{"hostId": idText(id), "intervalSeconds": 15}, e
}

func validateReport(v *hoststats.Report) error {
	if v.Protocol != 1 || !validToken(v.ReportID) || v.Hostname == "" || len(v.Hostname) > 180 || len(v.AgentVersion) > 40 {
		return bad("Agent 上报格式不正确")
	}
	if v.CollectedAt < now()-300000 || v.CollectedAt > now()+300000 {
		return bad("Agent 与服务端时钟偏差超过 5 分钟，请同步系统时间")
	}
	inv, snap := &v.Inventory, &v.Snapshot
	if len(inv.OS) > 180 || len(inv.Kernel) > 180 || len(inv.CPU) > 300 || len(inv.Arch) > 40 || inv.Cores < 0 || inv.Cores > 65536 || len(inv.IPs) > 64 || len(snap.Disks) > 32 || len(snap.Errors) > 16 {
		return bad("主机资料超过限制")
	}
	for _, p := range []*float64{snap.CPU, snap.Memory} {
		if p != nil && (math.IsNaN(*p) || math.IsInf(*p, 0) || *p < 0 || *p > 100) {
			return bad("无效的资源使用率")
		}
	}
	for _, p := range []*float64{snap.NetIn, snap.NetOut} {
		if p != nil && (math.IsNaN(*p) || math.IsInf(*p, 0) || *p < 0 || *p > 1e15) {
			return bad("无效的网络指标")
		}
	}
	capacityMounts := map[string]map[string]bool{}
	for _, d := range snap.Disks {
		if len(d.Path) > 512 || len(d.Filesystem) > 100 || math.IsNaN(d.Percent) || math.IsInf(d.Percent, 0) || d.Percent < 0 || d.Percent > 100 || (d.Used > d.Total && d.CapacityKind != "apfs-quota") {
			return bad("无效的磁盘指标")
		}
		if d.Path == "" || strings.ContainsRune(d.Path, '\x00') || len(d.CapacityID) > 180 || len(d.MountPoints) > 32 {
			return bad("无效的磁盘指标")
		}
		if d.CapacityID == "" {
			if d.CapacityKind != "" || len(d.MountPoints) > 0 {
				return bad("无效的磁盘指标")
			}
		} else if (d.CapacityKind != "apfs-container" && d.CapacityKind != "apfs-quota") || !strings.EqualFold(d.Filesystem, "apfs") || strings.TrimSpace(d.CapacityID) != d.CapacityID || strings.IndexFunc(d.CapacityID, unicode.IsControl) >= 0 {
			return bad("无效的磁盘指标")
		}
		if d.CapacityID != "" && (d.Total == 0 || (d.CapacityKind == "apfs-quota" && d.Used > d.Total && d.Percent != 100)) {
			return bad("无效的磁盘指标")
		}
		for _, mount := range d.MountPoints {
			if mount == "" || len(mount) > 512 || strings.ContainsRune(mount, '\x00') {
				return bad("无效的磁盘指标")
			}
		}
		if d.CapacityID != "" {
			key := d.CapacityKind + ":" + d.CapacityID
			if capacityMounts[key] == nil {
				capacityMounts[key] = map[string]bool{}
			}
			capacityMounts[key][d.Path] = true
			for _, mount := range d.MountPoints {
				capacityMounts[key][mount] = true
			}
			if len(capacityMounts[key]) > 32 {
				return bad("无效的磁盘指标")
			}
		}
	}
	for _, ip := range inv.IPs {
		if len(ip) > 100 {
			return bad("主机地址过长")
		}
	}
	for _, msg := range snap.Errors {
		if len(msg) > 512 {
			return bad("采集错误信息过长")
		}
	}
	inv.Source = "远程 Agent"
	if inv.IPs == nil {
		inv.IPs = []string{}
	}
	snap.Disks = hoststats.NormalizeDisks(snap.Disks)
	// Normalization canonicalizes UUIDs, so identities differing only in UUID
	// letter case can combine even when their raw-identity groups passed above.
	for _, disk := range snap.Disks {
		if len(disk.MountPoints) > 32 {
			return bad("无效的磁盘指标")
		}
	}
	if snap.Errors == nil {
		snap.Errors = []string{}
	}
	return nil
}

func (s *Server) reportAgent(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || !validToken(credential) {
		return nil, &apiError{401, "Agent 凭据无效或已撤销"}
	}
	var report hoststats.Report
	if e := decode(r, &report); e != nil {
		return nil, e
	}
	if e := validateReport(&report); e != nil {
		return nil, e
	}
	inv, _ := json.Marshal(report.Inventory)
	snap, _ := json.Marshal(report.Snapshot)
	e := s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var id, lastAt, lastSeen int64
		var lastID string
		e := tx.QueryRow("SELECT id,reported_at,last_seen,report_id FROM hosts WHERE source='agent' AND agent_enabled=1 AND token_hash=?", digest(credential)).Scan(&id, &lastAt, &lastSeen, &lastID)
		if e == sql.ErrNoRows {
			return &apiError{401, "Agent 凭据无效或已撤销"}
		}
		if e != nil {
			return e
		}
		if lastID == report.ReportID {
			return nil
		}
		if report.CollectedAt <= lastAt {
			return &apiError{409, "已收到更新的指标，上报已忽略"}
		}
		if now()-lastSeen < 5000 {
			return &apiError{429, "上报间隔不能短于 5 秒"}
		}
		at := now()
		if _, e = tx.Exec("UPDATE hosts SET hostname=?,inventory=?,snapshot=?,last_seen=?,reported_at=?,report_id=?,agent_version=? WHERE id=?", report.Hostname, string(inv), string(snap), at, report.CollectedAt, report.ReportID, report.AgentVersion, id); e != nil {
			return e
		}
		if _, e = tx.Exec("INSERT INTO host_samples(host_id,at,cpu,memory,net_in,net_out) VALUES(?,?,?,?,?,?)", id, at, report.Snapshot.CPU, report.Snapshot.Memory, report.Snapshot.NetIn, report.Snapshot.NetOut); e != nil {
			return e
		}
		if e = transitionAlert(tx, fmt.Sprintf("host:%d:offline", id), "host", id, report.Hostname, "", "Agent 已恢复上报", "", false); e != nil {
			return e
		}
		return evaluateHostAlerts(tx, id, report.Hostname, report.Snapshot, at)
	})
	return map[string]any{"accepted": true, "intervalSeconds": 15}, e
}

func (s *Server) detectOfflineAgents(ctx context.Context) error {
	return s.Store.write(ctx, func(tx *sql.Tx) error {
		rows, e := tx.Query("SELECT id,hostname FROM hosts WHERE source='agent' AND agent_enabled=1 AND enrolled_at>0 AND max(last_seen,enrolled_at)<?", now()-90000)
		if e != nil {
			return e
		}
		type item struct {
			id   int64
			name string
		}
		hosts := []item{}
		for rows.Next() {
			var h item
			if e = rows.Scan(&h.id, &h.name); e != nil {
				rows.Close()
				return e
			}
			hosts = append(hosts, h)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, h := range hosts {
			if e = transitionAlert(tx, fmt.Sprintf("host:%d:offline", h.id), "host", h.id, h.name, "远程主机离线", "超过 90 秒未收到 Agent 上报，请检查主机和网络", "critical", true); e != nil {
				return e
			}
		}
		return nil
	})
}
func (s *Server) agentLoop(ctx context.Context) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.detectOfflineAgents(ctx)
		}
	}
}
