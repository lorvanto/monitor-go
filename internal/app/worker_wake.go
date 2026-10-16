package app

import (
	"context"
	"database/sql"
	"time"
)

// Coalesced, post-commit hints; SQLite remains the durable queue of record.
func (s *Store) wakeWorkers() {
	for _, ch := range []chan struct{}{s.checkWake, s.notificationWake} {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

const workerFallback = 30 * time.Second

func dueDelay(at sql.NullInt64, err error) time.Duration {
	if err != nil {
		return time.Second
	}
	if !at.Valid {
		return workerFallback
	}
	return min(workerFallback, max(10*time.Millisecond, time.Until(time.UnixMilli(at.Int64))))
}
func (s *Server) checkDelay(ctx context.Context, available bool) time.Duration {
	if !available {
		return workerFallback
	}
	var next sql.NullInt64
	err := s.Store.DB.QueryRowContext(ctx, `SELECT min(at) FROM (
 SELECT 0 AS at FROM jobs WHERE state='queued'
 UNION ALL
 SELECT next_check AS at FROM monitors m WHERE enabled=1 AND deleted_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.monitor_id=m.id AND j.state IN ('queued','running'))
 )`).Scan(&next)
	return dueDelay(next, err)
}
func (s *Server) notificationDelay(ctx context.Context, available bool) time.Duration {
	if !available {
		return workerFallback
	}
	var next sql.NullInt64
	err := s.Store.DB.QueryRowContext(ctx, "SELECT min(next_attempt) FROM notification_deliveries WHERE state='queued'").Scan(&next)
	return dueDelay(next, err)
}

func workerRetryPause(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
