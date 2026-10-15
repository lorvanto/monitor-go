package app

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestHourlyRollupIsIdempotentAndPreservesLateDataAfterCleanup(t *testing.T) {
	s, _, c := fixture(t)
	ctx := context.Background()
	at := now()
	hour := at / hourMillis * hourMillis
	mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'history','HTTP','http://example.com','test',60,5),(2,'no samples','HTTP','http://example.com','test',60,5)")
	for _, sample := range []struct {
		at      int64
		state   string
		latency int
	}{{hour - 8*24*hourMillis, "healthy", 10}, {hour - 2*hourMillis - 1, "failed", 50}, {hour - 2*hourMillis, "warning", 30}, {hour - 2*hourMillis + 1, "healthy", 10}, {hour, "healthy", 20}} {
		mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,?,?,'test')", sample.at, sample.state, sample.latency)
	}
	before, err := s.availability(ctx, 720, at+1, "")
	if err != nil {
		t.Fatal(err)
	}
	if before.Samples != 5 || before.Uptime == nil || *before.Uptime != 80 || before.Latency == nil || *before.Latency != 24 {
		t.Fatalf("raw totals: %+v", before)
	}
	if err = s.rollupChecks(ctx, at); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.rollupChecks(ctx, at); err != nil {
			t.Fatal(err)
		}
	}
	after, err := s.availability(ctx, 720, at+1, "")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("rollup changed totals: before=%+v after=%+v %v", before, after, err)
	}
	var pending int
	s.Store.DB.QueryRow("SELECT count(*) FROM check_results WHERE rolled_up=0").Scan(&pending)
	if pending != 1 {
		t.Fatalf("current hour consumed: %d", pending)
	}
	mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'failed',60,'late')", hour-2*hourMillis+2)
	if err = s.rollupChecks(ctx, at); err != nil {
		t.Fatal(err)
	}
	after, err = s.availability(ctx, 720, at+1, "")
	if err != nil || after.Samples != 6 || math.Abs(*after.Uptime-100*4.0/6) > 1e-8 || *after.Latency != 30 {
		t.Fatalf("late result: %+v %v", after, err)
	}
	s.cleanup(ctx)
	var old int
	s.Store.DB.QueryRow("SELECT count(*) FROM check_results WHERE at<?", hour-7*24*hourMillis).Scan(&old)
	if old != 0 {
		t.Fatal("old raw results not cleaned")
	}
	retained, err := s.availability(ctx, 720, at+1, "")
	if err != nil || !reflect.DeepEqual(after, retained) {
		t.Fatalf("cleanup changed summaries: %+v %v", retained, err)
	}
	api := decodeValue[AvailabilityReport](t, c.call("GET", "/monitors/availability?hours=720", nil, 200))
	if api.Samples != 6 || len(api.Items) != 1 || api.Items[0].MonitorID != "1" {
		t.Fatalf("HTTP availability: %+v", api)
	}
	overview := decodeValue[struct {
		Samples int64
		Uptime  *float64
	}](t, c.call("GET", "/overview?hours=2160", nil, 200))
	if overview.Samples != 6 || overview.Uptime == nil {
		t.Fatal("overview lost summaries")
	}
	c.call("GET", "/monitors/availability?hours=24", nil, 422)
	empty, err := s.availability(ctx, 2160, at, "missing group")
	if err != nil || empty.Samples != 0 || empty.Uptime != nil || len(empty.Points) != 0 {
		t.Fatalf("missing history became healthy: %+v", empty)
	}
	mustSQL(t, s, "UPDATE monitors SET deleted_at=1 WHERE id=1")
	archived, err := s.availability(ctx, 720, at, "")
	if err != nil || archived.Samples != 0 {
		t.Fatal("archived monitor included")
	}
}
func TestHourlyRollupRollbackAndReusedIDs(t *testing.T) {
	s, _, _ := fixture(t)
	at := now()
	hour := at / hourMillis * hourMillis
	mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'rollback','TCP','example.com:443','test',60,5)")
	mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'healthy',10,'one')", hour-hourMillis)
	mustSQL(t, s, "CREATE TRIGGER reject_rollup BEFORE INSERT ON check_hourly BEGIN SELECT RAISE(ABORT,'test'); END")
	if err := s.rollupChecks(context.Background(), at); err == nil {
		t.Fatal("expected rollback")
	}
	var consumed int
	s.Store.DB.QueryRow("SELECT sum(rolled_up) FROM check_results").Scan(&consumed)
	if consumed != 0 {
		t.Fatal("failed rollup consumed raw rows")
	}
	mustSQL(t, s, "DROP TRIGGER reject_rollup")
	if err := s.rollupChecks(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	mustSQL(t, s, "DELETE FROM check_results")
	mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'failed',30,'two')", hour-hourMillis)
	if err := s.rollupChecks(context.Background(), at); err != nil {
		t.Fatal(err)
	}
	report, err := s.availability(context.Background(), 720, at, "")
	if err != nil || report.Samples != 2 || *report.Uptime != 50 || *report.Latency != 20 || *report.Items[0].Maximum != 30 {
		t.Fatalf("reused ID or weighted average: %+v %v", report, err)
	}
}
func TestMigrationEightRetentionConstraintsAndExpiry(t *testing.T) {
	dir, db := versionFourDatabase(t)
	if _, err := db.Exec(migration5 + migration6 + migration7); err != nil {
		t.Fatal(err)
	}
	db.Close()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := &Server{Store: store}
	settings, err := readSettings(store.DB)
	if err != nil || settings.RawRetentionDays != 7 || settings.SummaryRetentionDays != 90 {
		t.Fatalf("retention defaults: %+v %v", settings, err)
	}
	for _, q := range []string{"UPDATE system_settings SET raw_retention_days=0", "UPDATE system_settings SET raw_retention_days=31", "UPDATE system_settings SET summary_retention_days=89", "UPDATE system_settings SET summary_retention_days=731"} {
		if _, err = store.DB.Exec(q); err == nil {
			t.Fatalf("missing constraint: %s", q)
		}
	}
	mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'expiry','TCP','example.com:443','test',60,5)")
	hour := now() / hourMillis * hourMillis
	mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'healthy',10,'expired')", hour-91*24*hourMillis)
	s.cleanup(context.Background())
	var count int
	store.DB.QueryRow("SELECT (SELECT count(*) FROM check_hourly)+(SELECT count(*) FROM check_results)").Scan(&count)
	if count != 0 {
		t.Fatal("expired history retained")
	}
	rows, err := store.DB.Query("EXPLAIN QUERY PLAN SELECT * FROM check_results WHERE rolled_up=0 AND at<?", hour)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var a, b, c int
		var detail string
		rows.Scan(&a, &b, &c, &detail)
		plan += detail
	}
	if !strings.Contains(plan, "check_results_pending_rollup") {
		t.Fatal(plan)
	}
}

func TestHourlyAvailabilityRequiresMonitorPermission(t *testing.T) {
	_, ts, c := fixture(t)
	role := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]string{"name": "Files", "description": ""}, 201))
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"permissions": []string{"file:read"}, "version": 1}, 200)
	c.call("POST", "/users", userInput{Username: "history-denied", Name: "Reader", Password: "Read-password-123!", Roles: []string{role["id"]}}, 201)
	reader := client(t, ts.URL)
	reader.login("history-denied", "Read-password-123!")
	reader.call("GET", "/monitors/availability?hours=720", nil, 403)
	reader.call("GET", "/monitors/availability?hours=2160", nil, 403)
}
