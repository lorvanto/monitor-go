package app

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRecentTotalsMatchRawAcrossRollupBoundaries(t *testing.T) {
	s, _, _ := fixture(t)
	hour := now() / hourMillis * hourMillis
	mustSQL(t, s, `INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'one','TCP','localhost:80','',60,5),(2,'archived','TCP','localhost:80','other',60,5)`)
	for _, id := range []int{1, 2} {
		for i := int64(-26 * 60); i <= 60; i++ {
			state := "healthy"
			if i%3 == 0 {
				state = "failed"
			}
			if i%7 == 0 {
				state = "warning"
			}
			mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(?,?,?,?,?)", id, hour+i*60000, state, 20+i%10, "test")
		}
	}
	mustSQL(t, s, "UPDATE monitors SET deleted_at=1 WHERE id=2")
	check := func() {
		t.Helper()
		for _, cut := range []int64{hour - 24*hourMillis, hour - 24*hourMillis + 1, hour - 24*hourMillis + 3599999, hour + hourMillis + 1} {
			var wantGood, wantTotal int
			var wantLatency float64
			if e := s.Store.DB.QueryRow("SELECT coalesce(sum(state IN ('healthy','warning')),0),count(*),coalesce(sum(latency),0) FROM check_results WHERE monitor_id=1 AND at>=?", cut).Scan(&wantGood, &wantTotal, &wantLatency); e != nil {
				t.Fatal(e)
			}
			var good, total int
			var sum float64
			e := s.Store.reader().QueryRowContext(context.Background(), recentCheckTotals+"SELECT coalesce(sum(s.good),0),coalesce(sum(s.total),0),coalesce(sum(s.latency_sum),0) FROM samples s JOIN monitors m ON m.id=s.monitor_id WHERE m.deleted_at IS NULL", cut).Scan(&good, &total, &sum)
			if e != nil || good != wantGood || total != wantTotal || math.Abs(sum-wantLatency) > 1e-6 {
				t.Fatalf("cut=%d got=%d/%d/%f want=%d/%d/%f err=%v", cut, good, total, sum, wantGood, wantTotal, wantLatency, e)
			}
		}
	}
	check()
	if e := s.rollupChecks(context.Background(), hour); e != nil {
		t.Fatal(e)
	}
	check()
	mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'failed',99,'late')", hour-24*hourMillis+1)
	check()
	if e := s.rollupChecks(context.Background(), hour); e != nil {
		t.Fatal(e)
	}
	check()
}

func TestPollingPermissionsCompactAlertsAndActivity(t *testing.T) {
	s, ts, c := fixture(t)
	t.Logf("HTTP address: %s", ts.URL)
	mustSQL(t, s, "INSERT INTO alerts(id,fingerprint,resource_kind,resource_id,resource,title,description,severity,opened_at) VALUES(1,'test','host',1,'host','test','','warning',?)", now())
	mustSQL(t, s, "INSERT INTO alert_activity(alert_id,at,actor,message) VALUES(1,?,'test','activity')", now())
	type pollResult struct {
		User           *User
		Alerts         []Alert
		Hosts          []Host
		Monitors       []Monitor
		OpenAlertCount int
	}
	got := decodeValue[pollResult](t, c.call("GET", "/poll?page=overview", nil, 200))
	if got.User == nil || len(got.Alerts) != 1 || len(got.Alerts[0].Activity) != 0 || got.OpenAlertCount != 1 {
		t.Fatalf("poll %+v", got)
	}
	full := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	activity := decodeValue[[]Activity](t, c.call("GET", "/alerts/1/activity", nil, 200))
	if !reflect.DeepEqual(activity, full[0].Activity) {
		t.Fatal("activity changed")
	}
	c.call("GET", "/alerts/999/activity", nil, 404)
	c.call("GET", "/poll?page=invalid", nil, 422)
	role := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]string{"name": "Files only"}, 201))
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"version": 1, "permissions": []string{"file:read"}}, 200)
	c.call("POST", "/users", userInput{Username: "perf-reader", Name: "Reader", Password: "Read-password-123!", Roles: []string{role["id"]}}, 201)
	reader := client(t, ts.URL)
	reader.login("perf-reader", "Read-password-123!")
	denied := decodeValue[pollResult](t, reader.call("GET", "/poll?page=overview", nil, 200))
	if len(denied.Alerts)+len(denied.Hosts)+len(denied.Monitors)+denied.OpenAlertCount != 0 {
		t.Fatal("poll leaked resources")
	}
	reader.call("GET", "/alerts/1/activity", nil, 403)
	reader.call("GET", "/monitors/1/history", nil, 403)
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"version": 2, "permissions": []string{"monitor:read"}}, 200)
	reader.call("GET", "/poll?page=overview", nil, 401)
}

func TestOverviewPointQueriesPreserveGroups(t *testing.T) {
	s, _, _ := fixture(t)
	at := now()
	mustSQL(t, s, `INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'one','TCP','localhost:80','',60,5),(2,'two','TCP','localhost:80','other',60,5)`)
	for i := 0; i < 10; i++ {
		mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'healthy',10,''),(2,?,'failed',30,'')", at-int64(i)*60000, at-int64(i)*60000)
	}
	for _, tc := range []struct {
		query           string
		count           int
		uptime, latency float64
	}{{"", 20, 50, 20}, {"&ungrouped=1", 10, 100, 10}, {"&group=other", 10, 0, 30}} {
		raw, e := s.overview(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/?hours=24"+tc.query, nil), &User{Permissions: []string{"monitor:read"}})
		if e != nil {
			t.Fatal(e)
		}
		v := raw.(map[string]any)
		if v["samples"].(int) != tc.count || *v["uptime"].(*float64) != tc.uptime || *v["latency"].(*float64) != tc.latency {
			t.Fatalf("%s: %+v", tc.query, v)
		}
		for _, p := range v["points"].([]Point) {
			if p.Value == nil || *p.Value != tc.latency {
				t.Fatal("point changed")
			}
		}
	}
}

func TestCompactCheckSignalsAndOnDemandHistoryAreEquivalent(t *testing.T) {
	s, ts, c := fixture(t)
	t.Logf("HTTP address: %s", ts.URL)
	mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds) VALUES(1,'signals','TCP','localhost:80','',60,3)")
	for i := 0; i < 40; i++ {
		mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(1,?,'healthy',?,'Original diagnostic')", now()-int64(i/2)*1000, i)
	}
	full, e := s.monitors(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	compact, e := s.monitorsWithDetails(context.Background(), false)
	if e != nil {
		t.Fatal(e)
	}
	if len(compact) != 1 || len(compact[0].History) != 32 {
		t.Fatal("signals lost")
	}
	for i, h := range compact[0].History {
		want := full[0].History[i]
		if h.Detail != "" || h.At != want.At || h.Latency != want.Latency || h.State != want.State {
			t.Fatal("compact signal changed")
		}
	}
	details := decodeValue[[]CheckResult](t, c.call("GET", "/monitors/1/history", nil, 200))
	if !reflect.DeepEqual(details, full[0].History) {
		t.Fatal("diagnostics/order changed")
	}
	mustSQL(t, s, "UPDATE monitors SET deleted_at=1 WHERE id=1")
	c.call("GET", "/monitors/1/history", nil, 404)
}
