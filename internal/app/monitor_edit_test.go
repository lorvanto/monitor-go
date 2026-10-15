package app

import (
	"context"
	"testing"
)

func TestMonitorEditCredentialsConflictArchiveAndPermissions(t *testing.T) {
	s, ts, c := fixture(t)
	in := monitorEditInput{monitorInput: monitorPayload("127.0.0.1:3306"), Version: 1}
	in.Type = "MySQL"
	in.Username = "probe"
	in.Password = "original-secret"
	created := decodeValue[map[string]string](t, c.call("POST", "/monitors", in.monitorInput, 201))
	id := created["id"]
	c.call("POST", "/monitors/"+id+"/checks", map[string]any{}, 202)
	s.Store.DB.Exec("UPDATE monitors SET failures=3,state='failed' WHERE id=?", id)
	s.Store.DB.Exec("INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(?,?,'failed',1,'historic')", id, now())
	in.Password = ""
	in.Name = "Edited monitor"
	in.Target = "127.0.0.1:3307"
	in.Interval = 120
	in.Timeout = 4
	c.call("PUT", "/monitors/"+id, in, 200)
	check := func(want string, version int) {
		t.Helper()
		m, err := scanMonitor(s.Store.DB.QueryRow("SELECT "+monitorColumns+" FROM monitors WHERE id=?", id))
		if err != nil {
			t.Fatal(err)
		}
		secret, err := s.Store.decrypt(m.Secret)
		if err != nil || secret != want || m.Version != version {
			t.Fatalf("secret/version: %q %d %v", secret, m.Version, err)
		}
	}
	check("original-secret", 2)
	var name, target, state string
	var interval, timeout, failures, next int
	if err := s.Store.DB.QueryRow("SELECT name,target,interval_seconds,timeout_seconds,failures,state,next_check FROM monitors WHERE id=?", id).Scan(&name, &target, &interval, &timeout, &failures, &state, &next); err != nil {
		t.Fatal(err)
	}
	if name != in.Name || target != in.Target || interval != 120 || timeout != 4 || failures != 0 || state != "unknown" || next != 0 {
		t.Fatal("fields/reset not persisted")
	}
	var count int
	s.Store.DB.QueryRow("SELECT count(*) FROM jobs WHERE monitor_id=? AND state='cancelled'", id).Scan(&count)
	if count != 1 {
		t.Fatal("queue not cancelled")
	}
	s.Store.DB.QueryRow("SELECT count(*) FROM check_results WHERE monitor_id=?", id).Scan(&count)
	if count != 1 {
		t.Fatal("history lost")
	}
	s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action='编辑检测'").Scan(&count)
	if count != 1 {
		t.Fatal("edit audit missing")
	}
	c.call("PUT", "/monitors/"+id, in, 409)
	check("original-secret", 2)
	in.Version = 2
	in.Password = "replacement-secret"
	c.call("PUT", "/monitors/"+id, in, 200)
	check("replacement-secret", 3)
	in.Version = 3
	in.ClearPassword = true
	c.call("PUT", "/monitors/"+id, in, 200)
	check("", 4)
	in.Version = 4
	in.ClearPassword = false
	c.call("PUT", "/monitors/"+id, in, 200)
	check("replacement-secret", 5)
	in.Version = 5
	in.Type = "HTTP"
	in.Target = "https://example.com/"
	c.call("PUT", "/monitors/"+id, in, 200)
	check("", 6)
	var username string
	s.Store.DB.QueryRow("SELECT username FROM monitors WHERE id=?", id).Scan(&username)
	if username != "" {
		t.Fatal("credentials kept for HTTP")
	}
	c.call("POST", "/users", userInput{Username: "reader", Name: "Reader", Password: "Read-password-123!", Roles: []string{"3"}}, 201)
	reader := client(t, ts.URL)
	reader.login("reader", "Read-password-123!")
	reader.call("PUT", "/monitors/"+id, in, 403)
	reader.call("DELETE", "/monitors/"+id, map[string]int{"version": 6}, 403)
	c.call("DELETE", "/monitors/"+id, map[string]int{"version": 6}, 200)
	in.Version = 7
	c.call("PUT", "/monitors/"+id, in, 409)
	list, err := s.monitors(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("archive visible: %v", err)
	}
}
