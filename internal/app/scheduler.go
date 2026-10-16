package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const nextQueuedJobQuery = "SELECT id FROM jobs WHERE state='queued' ORDER BY id LIMIT 1"

func (s *Server) scheduler(ctx context.Context) {
	var workers sync.WaitGroup
	defer workers.Wait()
	s.cleanupUploadResidue(ctx)
	s.collectPurgedFiles(ctx)
	if err := s.rollupChecks(ctx, now()); err != nil {
		slog.Error("Metric rollup failed", "error", err)
	}
	slots := make(chan struct{}, 4)
	tick := time.NewTimer(0)
	defer tick.Stop()
	maintenance := time.NewTicker(time.Hour)
	defer maintenance.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-maintenance.C:
			s.cleanup(ctx)
			continue
		case <-s.Store.checkWake:
		case <-tick.C:
		}
		rows, e := s.Store.DB.QueryContext(ctx, "SELECT id FROM monitors m WHERE enabled=1 AND deleted_at IS NULL AND next_check<=? AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.monitor_id=m.id AND j.state IN ('queued','running')) ORDER BY next_check,id LIMIT 64", now())
		if e != nil {
			tick.Reset(time.Second)
			continue
		}
		ids := []int64{}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			if _, e := s.enqueue(ctx, id); e != nil {
				slog.Debug("检测调度延后", "error", e)
			}
		}
		for len(slots) < cap(slots) {
			var id int64
			e = s.Store.DB.QueryRowContext(ctx, nextQueuedJobQuery).Scan(&id)
			if e != nil {
				break
			}
			res, e := s.Store.DB.ExecContext(ctx, "UPDATE jobs SET state='running' WHERE id=? AND state='queued'", id)
			if e != nil {
				break
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				continue
			}
			slots <- struct{}{}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-slots; s.Store.wakeWorkers() }()
				if e := s.execute(ctx, id); e != nil {
					slog.Error("检测执行入库失败", "job", id, "error", e)
					s.Store.DB.Exec("UPDATE jobs SET state='queued' WHERE id=? AND state='running'", id)
					workerRetryPause(ctx)
				}
			}()
		}
		tick.Reset(s.checkDelay(ctx, len(slots) < cap(slots)))
	}
}
func (s *Server) execute(ctx context.Context, job int64) error {
	var monitorID int64
	if e := s.Store.DB.QueryRowContext(ctx, "SELECT monitor_id FROM jobs WHERE id=?", job).Scan(&monitorID); e != nil {
		return e
	}
	m, e := scanMonitor(s.Store.DB.QueryRowContext(ctx, "SELECT "+monitorColumns+" FROM monitors WHERE id=? AND deleted_at IS NULL", monitorID))
	if e == sql.ErrNoRows || (!m.Enabled && e == nil) {
		_, e = s.Store.DB.ExecContext(ctx, "UPDATE jobs SET state='cancelled',finished_at=? WHERE id=?", now(), job)
		return e
	}
	if e != nil {
		return e
	}
	password, e := s.Store.decrypt(m.Secret)
	var result CheckResult
	if e != nil {
		result = CheckResult{At: now(), State: "failed", Detail: "检测凭据无法解密"}
	} else {
		probeCtx, cancel := context.WithTimeout(ctx, time.Duration(m.Timeout)*time.Second)
		result = probe(probeCtx, m, password)
		cancel()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return s.Store.write(ctx, func(tx *sql.Tx) error {
		var version, failures int
		var enabled bool
		var deleted sql.NullInt64
		if e := tx.QueryRow("SELECT version,enabled,deleted_at,failures FROM monitors WHERE id=?", m.ID).Scan(&version, &enabled, &deleted, &failures); e != nil {
			return e
		}
		if !enabled || deleted.Valid || version != m.Version {
			_, e := tx.Exec("UPDATE jobs SET state='cancelled',finished_at=? WHERE id=?", now(), job)
			return e
		}
		if result.State == "failed" || result.State == "warning" {
			failures++
		} else {
			failures = 0
		}
		if _, e := tx.Exec("INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(?,?,?,?,?)", m.ID, result.At, result.State, result.Latency, result.Detail); e != nil {
			return e
		}
		if _, e := tx.Exec("UPDATE monitors SET state=?,latency=?,detail=?,checked_at=?,failures=? WHERE id=?", result.State, result.Latency, result.Detail, result.At, failures, m.ID); e != nil {
			return e
		}
		state := "succeeded"
		if result.State == "failed" {
			state = "failed"
		}
		if _, e := tx.Exec("UPDATE jobs SET state=?,finished_at=? WHERE id=?", state, now(), job); e != nil {
			return e
		}
		fingerprint := fmt.Sprintf("monitor:%d", m.ID)
		if failures >= m.Threshold {
			severity := "critical"
			title := fmt.Sprintf("%s 连续 %d 次检测异常", m.Name, failures)
			if result.State == "warning" {
				severity = "warning"
				title = m.Name + " response time exceeds the configured threshold"
				if m.Type == "SSL" {
					title = m.Name + " 证书即将到期"
				}
			}
			return transitionAlert(tx, fingerprint, "monitor", m.ID, m.Name, title, result.Detail, severity, true)
		}
		if result.State == "healthy" {
			return transitionAlert(tx, fingerprint, "monitor", m.ID, m.Name, "", result.Detail, "", false)
		}
		return nil
	})
}
func (s *Server) cleanup(ctx context.Context) {
	s.cleanupUploadResidue(ctx)
	s.collectPurgedFiles(ctx)
	settings, err := readSettings(s.Store.DB)
	if err != nil {
		return
	}
	at := now()
	if err = s.rollupChecks(ctx, at); err != nil {
		slog.Error("Metric rollup failed; keeping raw data", "error", err)
		return
	}
	rawCutoff := (at - int64(settings.RawRetentionDays)*86400000) / hourMillis * hourMillis
	summaryCutoff := (at - int64(settings.SummaryRetentionDays)*86400000) / hourMillis * hourMillis
	deadline := time.Now().Add(10 * time.Second)
	queries := []struct {
		sql    string
		cutoff int64
	}{
		{"DELETE FROM audit_logs WHERE id IN (SELECT id FROM audit_logs WHERE at < ? LIMIT 5000)", now() - int64(settings.AuditDays)*86400000},
		{"DELETE FROM notification_deliveries WHERE id IN (SELECT id FROM notification_deliveries WHERE finished_at < ? AND state NOT IN ('queued','sending') LIMIT 5000)", now() - 30*86400000},
		{"DELETE FROM sessions WHERE token_hash IN (SELECT token_hash FROM sessions WHERE expires_at < ? LIMIT 5000)", now()},
		{"DELETE FROM host_samples WHERE id IN (SELECT id FROM host_samples WHERE at < ? LIMIT 5000)", rawCutoff},
		{"DELETE FROM check_results WHERE id IN (SELECT id FROM check_results WHERE rolled_up=1 AND at < ? LIMIT 5000)", rawCutoff},
		{"DELETE FROM check_hourly WHERE rowid IN (SELECT rowid FROM check_hourly WHERE hour < ? LIMIT 5000)", summaryCutoff},
		{"DELETE FROM jobs WHERE id IN (SELECT id FROM jobs WHERE finished_at < ? AND state NOT IN ('queued','running') LIMIT 5000)", now() - 7*86400000},
	}
	for _, q := range queries {
		for batch := 0; batch < 200 && time.Now().Before(deadline) && ctx.Err() == nil; batch++ {
			res, err := s.Store.DB.ExecContext(ctx, q.sql, q.cutoff)
			if err != nil {
				slog.Error("历史清理失败", "error", err)
				return
			}
			n, _ := res.RowsAffected()
			if n < 5000 {
				break
			}
		}
	}
}
