package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/hoststats"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type inventory = hoststats.Inventory
type snapshot = hoststats.Snapshot
type diskInfo = hoststats.DiskInfo

func (s *Server) collectLoop(ctx context.Context) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		if e := s.collect(ctx); e != nil && ctx.Err() == nil {
			slog.Error("主机采样失败", "error", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Server) collect(ctx context.Context) error {
	report, e := s.collector.Collect(ctx)
	if e != nil {
		return e
	}
	inv, snap := report.Inventory, report.Snapshot
	rawInv, e := json.Marshal(inv)
	if e != nil {
		return e
	}
	rawSnap, e := json.Marshal(snap)
	if e != nil {
		return e
	}
	return s.Store.write(ctx, func(tx *sql.Tx) error {
		if _, e := tx.Exec(`INSERT INTO hosts(id,hostname,alias,inventory,snapshot,last_seen) VALUES(1,?,'本机',?,?,?) ON CONFLICT(id) DO UPDATE SET hostname=excluded.hostname,inventory=excluded.inventory,snapshot=excluded.snapshot,last_seen=excluded.last_seen`, report.Hostname, string(rawInv), string(rawSnap), now()); e != nil {
			return e
		}
		if _, e := tx.Exec("INSERT INTO host_samples(host_id,at,cpu,memory,net_in,net_out) VALUES(1,?,?,?,?,?)", now(), snap.CPU, snap.Memory, snap.NetIn, snap.NetOut); e != nil {
			return e
		}
		details := []string{}
		for _, d := range snap.Disks {
			if d.Percent >= 85 {
				details = append(details, fmt.Sprintf("%s 使用率 %.1f%%", d.Path, d.Percent))
			}
		}
		if len(snap.Disks) > 0 {
			if e := transitionAlert(tx, "host:1:disk", "host", 1, report.Hostname, "主机磁盘空间需要关注", strings.Join(details, "；"), "warning", len(details) > 0); e != nil {
				return e
			}
		}
		return nil
	})
}
func (s *Server) hosts(ctx context.Context) ([]Host, error) {
	out := []Host{}
	rows, e := s.Store.DB.QueryContext(ctx, "SELECT id,hostname,alias,inventory,snapshot,last_seen,version,source,agent_enabled,agent_version,enrolled_at,enrollment_expires FROM hosts ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var h Host
		var inv, snap string
		if e = rows.Scan(&h.ID, &h.Hostname, &h.Alias, &inv, &snap, &h.LastSeen, &h.Version, &h.Source, &h.AgentEnabled, &h.AgentVersion, &h.EnrolledAt, &h.EnrollmentExpires); e != nil {
			return nil, e
		}
		h.Status = "online"
		if h.Source == "agent" && !h.AgentEnabled {
			h.Status = "revoked"
		} else if h.Source == "agent" && (h.EnrolledAt == 0 || h.LastSeen < h.EnrolledAt) {
			h.Status = "pending"
			if h.EnrolledAt > 0 && now()-h.EnrolledAt > 90000 {
				h.Status = "offline"
			}
		} else if h.LastSeen == 0 {
			h.Status = "pending"
		} else if now()-h.LastSeen > 90000 {
			h.Status = "offline"
		}
		h.Inventory = json.RawMessage(inv)
		h.Snapshot = json.RawMessage(snap)
		out = append(out, h)
	}
	return out, rows.Err()
}
func (s *Server) listHosts(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return s.hosts(r.Context())
}
func (s *Server) updateHost(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Alias   string `json:"alias"`
		Version int    `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	in.Alias = strings.TrimSpace(in.Alias)
	if len(in.Alias) > 180 {
		return nil, bad("主机备注过长")
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		res, e := tx.Exec("UPDATE hosts SET alias=?,version=version+1 WHERE id=? AND version=?", in.Alias, id, in.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict()
		}
		return auditRequest(tx, r, u.Name, "更新主机资料", idText(id))
	})
	return true, e
}
func (s *Server) hostMetrics(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	bucket := rangeHours(r) * 3600000 / 180
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT (at/?)*?,avg(cpu),avg(memory),avg(net_in),avg(net_out) FROM host_samples WHERE host_id=? AND at>=? GROUP BY at/? ORDER BY at/?", bucket, bucket, id, since(r), bucket, bucket)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	type metric struct {
		At     int64    `json:"at"`
		CPU    *float64 `json:"cpu"`
		Memory *float64 `json:"memory"`
		NetIn  *float64 `json:"netIn"`
		NetOut *float64 `json:"netOut"`
	}
	out := []metric{}
	for rows.Next() {
		var m metric
		if e = rows.Scan(&m.At, &m.CPU, &m.Memory, &m.NetIn, &m.NetOut); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
