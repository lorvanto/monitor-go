package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Pre-T7 implementation retained only as an equivalence and benchmark oracle.
func (s *Server) legacyMonitors(ctx context.Context) ([]Monitor, error) {
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
	for i := range out {
		m := &out[i]
		var good, total int
		e = s.Store.DB.QueryRowContext(ctx, "SELECT coalesce(sum(state IN ('healthy','warning')),0),count(*) FROM check_results WHERE monitor_id=? AND at>=?", m.ID, now()-86400000).Scan(&good, &total)
		if e != nil {
			return nil, e
		}
		if total > 0 {
			v := float64(good) * 100 / float64(total)
			m.Uptime = &v
		}
		rows, e = s.Store.DB.QueryContext(ctx, "SELECT at,state,latency,detail FROM check_results WHERE monitor_id=? ORDER BY at DESC,id DESC LIMIT 32", m.ID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var v CheckResult
			if e = rows.Scan(&v.At, &v.State, &v.Latency, &v.Detail); e != nil {
				rows.Close()
				return nil, e
			}
			m.History = append(m.History, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *Server) legacyAlerts(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := []Alert{}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT a.id,a.resource_kind,a.resource_id,a.resource,a.title,a.description,a.severity,a.state,a.opened_at,a.recovered_at,coalesce(u.display_name,''),a.silence_until,a.version FROM alerts a LEFT JOIN users u ON u.id=a.ack_by ORDER BY a.state='resolved',a.opened_at DESC LIMIT 200`)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		a := Alert{Activity: []Activity{}}
		if e = rows.Scan(&a.ID, &a.Kind, &a.ResourceID, &a.Resource, &a.Title, &a.Description, &a.Severity, &a.State, &a.Opened, &a.Recovered, &a.Owner, &a.SilenceUntil, &a.Version); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		rows, e = s.Store.DB.QueryContext(r.Context(), "SELECT at,actor,message FROM alert_activity WHERE alert_id=? ORDER BY id DESC LIMIT 100", out[i].ID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var a Activity
			if e = rows.Scan(&a.At, &a.Actor, &a.Message); e != nil {
				rows.Close()
				return nil, e
			}
			out[i].Activity = append(out[i].Activity, a)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

func TestBatchQueryEquivalenceAndPlans(t *testing.T) {
	s, _, c := fixture(t)
	for i := 1; i <= 8; i++ {
		mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(?,?,'TCP','example.com:443','test',60,5)", i, fmt.Sprint(i))
		for n := 0; n < i*10; n++ {
			state := "healthy"
			if n%3 == 0 {
				state = "failed"
			}
			if n%5 == 0 {
				state = "warning"
			}
			// Paired timestamps exercise the id DESC tie-breaker.
			mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(?,?,?,?,?)", i, now()-int64(n/2)*60000, state, n, fmt.Sprint(n))
		}
		mustSQL(t, s, "INSERT INTO alerts(id,fingerprint,resource_kind,resource_id,resource,title,description,severity,opened_at) VALUES(?,?,'monitor',?,'test','test','','warning',?)", i, fmt.Sprint(i), i, now())
		for n := 0; n < 120; n++ {
			mustSQL(t, s, "INSERT INTO alert_activity(alert_id,at,actor,message) VALUES(?,?,'test',?)", i, now(), fmt.Sprint(n))
		}
	}
	mustSQL(t, s, "UPDATE monitors SET deleted_at=1 WHERE id=8")
	mustSQL(t, s, "UPDATE monitors SET enabled=0 WHERE id=7")
	mustSQL(t, s, "DELETE FROM check_results WHERE monitor_id=6")
	old, err := s.legacyMonitors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.monitors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, old) {
		t.Fatal("monitor response changed")
	}
	req := httptest.NewRequest("GET", "/", nil)
	oldAlerts, err := s.legacyAlerts(nil, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotAlerts, err := s.listAlerts(nil, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotAlerts, oldAlerts) {
		t.Fatal("alert response changed")
	}
	summary := decodeValue[map[string]int](t, c.call("GET", "/alerts/summary", nil, 200))
	if summary["open"] != 8 {
		t.Fatalf("summary: %v", summary)
	}
	for _, query := range []struct {
		sql  string
		args []any
	}{{monitorUptimeQuery, []any{now() - 86400000}}, {monitorHistoryQuery, nil}} {
		rows, err := s.Store.DB.Query("EXPLAIN QUERY PLAN "+query.sql, query.args...)
		if err != nil {
			t.Fatal(err)
		}
		plans := []string{}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plans = append(plans, detail)
		}
		rows.Close()
		if !strings.Contains(strings.Join(plans, ";"), "check_results_time") {
			t.Fatalf("missing indexed search: %v", plans)
		}
		t.Log(strings.Join(plans, "; "))
	}
}

func BenchmarkMonitorList(b *testing.B) {
	store, err := OpenStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	_, err = store.DB.Exec(`WITH RECURSIVE nums(n) AS(VALUES(1) UNION ALL SELECT n+1 FROM nums WHERE n<500)
 INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) SELECT n,'benchmark-'||n,'TCP','example.com:443','test',60,5 FROM nums`)
	if err != nil {
		b.Fatal(err)
	}
	_, err = store.DB.Exec(`WITH RECURSIVE minutes(n) AS(VALUES(0) UNION ALL SELECT n+1 FROM minutes WHERE n<1439)
 INSERT INTO check_results(monitor_id,at,state,latency,detail) SELECT m.id,?-n*60000,CASE WHEN n%17=0 THEN 'failed' ELSE 'healthy' END,12,'benchmark sample' FROM monitors m CROSS JOIN minutes`, now())
	if err != nil {
		b.Fatal(err)
	}
	s := &Server{Store: store}
	for _, tc := range []struct {
		name string
		load func(context.Context) ([]Monitor, error)
	}{{"legacy", s.legacyMonitors}, {"batched", s.monitors}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := tc.load(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
