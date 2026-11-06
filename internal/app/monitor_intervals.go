package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Period changes are one business transaction: either every selected live target
// is valid and changed, or none of them are. Configuration versions invalidate
// results from probes that were already running with the previous schedule.
func (s *Server) updateMonitorIntervals(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		IDs      []string `json:"ids"`
		Interval int      `json:"interval"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if len(in.IDs) < 1 || len(in.IDs) > 500 {
		return nil, bad("请选择 1–500 个检测目标")
	}
	if in.Interval < 15 || in.Interval > 86400 {
		return nil, bad("检测周期需要为 15–86400 秒")
	}
	ids := make([]int64, len(in.IDs))
	seen := make(map[int64]bool, len(ids))
	for i, raw := range in.IDs {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 || idText(id) != raw || seen[id] {
			return nil, bad("检测目标编号无效或重复")
		}
		ids[i], seen[id] = id, true
	}
	err := s.Store.write(r.Context(), func(tx *sql.Tx) error {
		at := now()
		nextChecks := make([]int64, len(ids))
		for i, id := range ids {
			var timeout, failures int
			var checked, next int64
			var kind, state string
			err := tx.QueryRow("SELECT timeout_seconds,checked_at,next_check,type,state,failures FROM monitors WHERE id=? AND deleted_at IS NULL", id).Scan(&timeout, &checked, &next, &kind, &state, &failures)
			if err == sql.ErrNoRows {
				return conflict()
			}
			if err != nil {
				return err
			}
			if in.Interval <= timeout {
				return bad("检测周期必须大于所选目标的超时")
			}
			nextChecks[i] = max(at, checked+int64(in.Interval)*1000)
			// Changing a daily period must not postpone an in-progress SSL
			// confirmation sequence. Paused targets remain paused throughout.
			if kind == "SSL" && state == "failed" && failures > 0 && failures < sslFailureThreshold {
				// enqueue may already have replaced next_check with the normal
				// period for an active job. The last failed observation still
				// bounds its confirmation deadline even if that job is cancelled.
				retry := min(next, checked+sslFailureRetry.Milliseconds())
				nextChecks[i] = min(nextChecks[i], max(at, retry))
			}
		}
		for i, id := range ids {
			if _, err := tx.Exec("UPDATE monitors SET interval_seconds=?,next_check=?,version=version+1 WHERE id=?", in.Interval, nextChecks[i], id); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE jobs SET state='cancelled',finished_at=? WHERE monitor_id=? AND state='queued'", at, id); err != nil {
				return err
			}
		}
		return auditRequest(tx, r, u.Name, "批量修改检测周期", fmt.Sprintf("%ds: %s", in.Interval, strings.Join(in.IDs, ",")))
	})
	return map[string]any{"ok": err == nil, "updated": len(ids)}, err
}
