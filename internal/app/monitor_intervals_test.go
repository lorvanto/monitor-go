package app

import (
	"context"
	"testing"
)

func TestMonitorBulkIntervalKeepsHistoryAndScopesChanges(t *testing.T) {
	s, _, c := fixture(t)
	first := monitorPayload("example.com:443")
	first.Type = "SSL"
	first.Threshold = 1 // SSL now uses the fixed short-recheck policy.
	sslID := decodeValue[map[string]string](t, c.call("POST", "/monitors", first, 201))["id"]
	second := monitorPayload("127.0.0.1:3306")
	second.Type, second.Username, second.Password = "MySQL", "probe", "synthetic-secret"
	dbID := decodeValue[map[string]string](t, c.call("POST", "/monitors", second, 201))["id"]
	otherID := decodeValue[map[string]string](t, c.call("POST", "/monitors", monitorPayload("https://example.com/"), 201))["id"]
	job := decodeValue[map[string]string](t, c.call("POST", "/monitors/"+sslID+"/checks", map[string]any{}, 202))["jobId"]
	at, expiry := now(), now()+10*86400000
	mustSQL(t, s, "UPDATE monitors SET state='failed',failures=2,checked_at=?,next_check=?,certificate_expires_at=? WHERE id=?", at, at+60000, expiry, sslID)
	mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail,certificate_expires_at) VALUES(?,?,'warning',5,'historic',?)", sslID, at, expiry)
	mustSQL(t, s, "UPDATE monitors SET enabled=0,state='healthy' WHERE id=?", dbID)
	var encrypted string
	s.Store.DB.QueryRow("SELECT secret FROM monitors WHERE id=?", dbID).Scan(&encrypted)
	result := decodeValue[struct {
		OK      bool `json:"ok"`
		Updated int  `json:"updated"`
	}](t, c.call("POST", "/monitors/interval", map[string]any{"ids": []string{sslID, dbID}, "interval": 86400}, 200))
	if !result.OK || result.Updated != 2 {
		t.Fatal("bulk result", result)
	}
	items, err := s.monitors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range items {
		switch idText(m.ID) {
		case sslID:
			if m.Interval != 86400 || m.Threshold != 3 || m.Version != 2 || m.State != "failed" || m.NextCheck != at+60000 || m.CertificateExpiresAt != expiry || len(m.History) != 1 || m.History[0].CertificateExpiresAt != expiry {
				t.Fatalf("SSL configuration, pending retry or history changed: %+v", m)
			}
		case dbID:
			if m.Enabled || m.Interval != 86400 || m.Secret != encrypted || m.Version != 2 {
				t.Fatal("credentials or pause state changed", m)
			}
		case otherID:
			if m.Interval != 60 || m.Version != 1 {
				t.Fatal("unselected target changed", m)
			}
		}
	}
	var state string
	if err = s.Store.DB.QueryRow("SELECT state FROM jobs WHERE id=?", job).Scan(&state); err != nil || state != "cancelled" {
		t.Fatal("queued old schedule not cancelled", state, err)
	}
	var audits int
	s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action='批量修改检测周期'").Scan(&audits)
	if audits != 1 {
		t.Fatal("bulk operation missing one audit event", audits)
	}
	// A stale editor cannot overwrite the new interval.
	c.call("PUT", "/monitors/"+sslID, monitorEditInput{monitorInput: first, Version: 1}, 409)
}

func TestMonitorBulkIntervalValidatesAtomicallyAndRequiresPermission(t *testing.T) {
	s, ts, c := fixture(t)
	in := monitorPayload("https://example.com/")
	in.Timeout = 20
	id := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))["id"]
	for _, invalid := range []map[string]any{
		{"ids": []string{}, "interval": 60},
		{"ids": []string{id, id}, "interval": 60},
		{"ids": []string{"+1"}, "interval": 60},
		{"ids": []string{"9223372036854775808"}, "interval": 60},
		{"ids": []string{id}, "interval": 14},
		{"ids": []string{id}, "interval": 86401},
		{"ids": []string{id}, "interval": 20},
		{"ids": []string{id}, "interval": 15},
	} {
		c.call("POST", "/monitors/interval", invalid, 422)
	}
	c.call("POST", "/monitors/interval", map[string]any{"ids": []string{id, "99999"}, "interval": 180}, 409)
	mustSQL(t, s, "CREATE TRIGGER reject_bulk_audit BEFORE INSERT ON audit_logs WHEN NEW.action='批量修改检测周期' BEGIN SELECT RAISE(ABORT,'test rollback'); END")
	c.call("POST", "/monitors/interval", map[string]any{"ids": []string{id}, "interval": 180}, 500)
	mustSQL(t, s, "DROP TRIGGER reject_bulk_audit")
	var interval, version, audits int
	s.Store.DB.QueryRow("SELECT interval_seconds,version FROM monitors WHERE id=?", id).Scan(&interval, &version)
	s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action='批量修改检测周期'").Scan(&audits)
	if interval != 60 || version != 1 || audits != 0 {
		t.Fatal("failed batch partly committed", interval, version, audits)
	}
	unauthenticated := client(t, ts.URL)
	unauthenticated.call("POST", "/monitors/interval", map[string]any{"ids": []string{id}, "interval": 180}, 401)
	c.call("POST", "/users", userInput{Username: "reader", Name: "Reader", Password: "Read-password-123!", Roles: []string{"3"}}, 201)
	reader := client(t, ts.URL)
	reader.login("reader", "Read-password-123!")
	reader.call("POST", "/monitors/interval", map[string]any{"ids": []string{id}, "interval": 180}, 403)
	csrf := c.csrf
	c.csrf = ""
	c.call("POST", "/monitors/interval", map[string]any{"ids": []string{id}, "interval": 180}, 403)
	c.csrf = csrf
	c.call("POST", "/monitors/interval", map[string]any{"ids": []string{id}, "interval": 180}, 200)
}

func TestMonitorBulkIntervalKeepsDueSSLRecheck(t *testing.T) {
	for _, jobState := range []string{"queued", "running"} {
		t.Run(jobState, func(t *testing.T) {
			s, _, c := fixture(t)
			in := monitorPayload("example.com:443")
			in.Type, in.Interval = "SSL", 86400
			id := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))["id"]
			var monitorID int64
			s.Store.DB.QueryRow("SELECT id FROM monitors WHERE id=?", id).Scan(&monitorID)
			first, err := s.enqueue(context.Background(), monitorID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := scanMonitor(s.Store.DB.QueryRow("SELECT "+monitorColumns+" FROM monitors WHERE id=?", id))
			if err != nil {
				t.Fatal(err)
			}
			if err = s.completeCheck(context.Background(), first, m, CheckResult{At: now() - 61000, State: "failed", Detail: "synthetic connection failure"}); err != nil {
				t.Fatal(err)
			}
			mustSQL(t, s, "UPDATE monitors SET next_check=? WHERE id=?", now()-1000, id)
			// The scheduler enqueues the due retry and provisionally advances
			// next_check by a whole day before the bulk change arrives.
			retry, err := s.enqueue(context.Background(), monitorID)
			if err != nil {
				t.Fatal(err)
			}
			mustSQL(t, s, "UPDATE jobs SET state=? WHERE id=?", jobState, retry)
			before := now()
			c.call("POST", "/monitors/interval", map[string]any{"ids": []string{id}, "interval": 86400}, 200)
			var next int64
			if err = s.Store.DB.QueryRow("SELECT next_check FROM monitors WHERE id=?", id).Scan(&next); err != nil || next < before || next > now()+1000 {
				t.Fatal("due confirmation postponed by bulk change", next-before, err)
			}
			// An obsolete running result cannot replace the failed observation.
			if err = s.completeCheck(context.Background(), retry, m, CheckResult{At: now(), State: "healthy"}); err != nil {
				t.Fatal(err)
			}
			var state string
			var failures, results int
			if err = s.Store.DB.QueryRow("SELECT state,failures,(SELECT count(*) FROM check_results WHERE monitor_id=monitors.id) FROM monitors WHERE id=?", id).Scan(&state, &failures, &results); err != nil || state != "failed" || failures != 1 || results != 1 {
				t.Fatal("obsolete retry changed observed state", state, failures, results, err)
			}
		})
	}
}
