package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func usageFixture(t *testing.T, origin string) (*Server, *testClient, func()) {
	t.Helper()
	s, err := New(Config{DataDir: t.TempDir(), Listen: "127.0.0.1:5080", ConsoleURL: origin})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	c := client(t, ts.URL)
	c.call("POST", "/setup", map[string]string{"token": s.setupToken, "username": "admin", "name": "Example owner", "password": "Example-password-123!"}, 200)
	u := decodeValue[User](t, c.call("GET", "/me", nil, 200))
	c.csrf = u.CSRF
	var cancel context.CancelFunc
	var done chan struct{}
	start := func() {
		if cancel != nil {
			t.Fatal("worker started twice")
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		done = make(chan struct{})
		go func() { defer close(done); s.usage.loop(ctx) }()
	}
	t.Cleanup(func() {
		ts.Close()
		if cancel != nil {
			cancel()
			<-done
		}
		s.Store.Close()
	})
	return s, c, start
}
func usageStateForTest(t *testing.T, s *Server) usageState {
	t.Helper()
	st, err := readUsageState(context.Background(), s.Store.DB)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
func waitUsage(t *testing.T, condition func() bool) {
	t.Helper()
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("usage condition was not reached")
}
func usageConfirmation(t *testing.T, c *testClient) map[string]any {
	status := decodeValue[usageStatus](t, c.call("GET", "/usage", nil, 200))
	return map[string]any{"enabled": true, "usageConsent": true, "usageNoticeVersion": "2", "usageConsoleUrl": status.ConsoleURL}
}
func enableUsage(t *testing.T, c *testClient) usageStatus {
	return decodeValue[usageStatus](t, c.call("PATCH", "/usage", usageConfirmation(t, c), 200))
}
func rejectUsageDisable(t *testing.T, c *testClient) {
	t.Helper()
	req, _ := http.NewRequest("PATCH", c.base+"/api/v1/usage", bytes.NewBufferString(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	res, err := c.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Error struct {
			Key string `json:"key"`
		} `json:"error"`
	}
	if err = json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 403 || body.Error.Key != usageRequired {
		t.Fatalf("disable did not return required-reporting error: status %d key %q", res.StatusCode, body.Error.Key)
	}
}
func fakeUsageConsole(t *testing.T) (*httptest.Server, *atomic.Int64, *atomic.Int64, *sync.Map) {
	t.Helper()
	registered, reports := &atomic.Int64{}, &atomic.Int64{}
	values := &sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		id, _ := in["instanceId"].(string)
		if len(id) != 32 || len(in) != 4 {
			t.Errorf("unexpected anonymous fields: %#v", in)
		}
		if r.Header.Get("User-Agent") != "Monitor-Go" {
			t.Error("usage User-Agent disclosed a build version")
		}
		switch r.URL.Path {
		case "/api/v2/instances/register":
			registered.Add(1)
			secret, _ := in["credential"].(string)
			if len(secret) != 64 {
				t.Error("missing independent installation credential")
			}
			values.Store("credential", secret)
			if _, ok := in["activeDays"]; ok {
				t.Error("registration included activity")
			}
		case "/api/v2/instances/report":
			stored, ok := values.Load("credential")
			if !ok || r.Header.Get("Authorization") != "Bearer "+stored.(string) {
				w.WriteHeader(401)
				return
			}
			reports.Add(1)
			values.Store("report", in)
		default:
			t.Error("anonymous client attempted an unsupported endpoint")
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(usageResult{InstanceID: id, FirstSeen: time.Now().Unix(), LastSeen: time.Now().Unix()})
	}))
	t.Cleanup(server.Close)
	return server, registered, reports, values
}

func TestUsageLegacyOffPreservesChoiceAndPermissions(t *testing.T) {
	central, registered, reports, _ := fakeUsageConsole(t)
	s, c, start := usageFixture(t, central.URL)
	start()
	status := decodeValue[usageStatus](t, c.call("GET", "/usage", nil, 200))
	if status.Enabled || !status.Required || status.NoticeVersion != "2" || !status.ConfirmationRequired || !status.Configured || status.InstanceID != "" || len(status.PendingActiveDays) != 0 {
		t.Fatalf("unsafe default %+v", status)
	}
	c.call("POST", "/usage/activity", map[string]any{}, 200)
	time.Sleep(100 * time.Millisecond)
	if registered.Load() != 0 || reports.Load() != 0 {
		t.Fatal("disabled instance contacted console")
	}
	var days int
	s.Store.DB.QueryRow("SELECT count(*) FROM usage_activity").Scan(&days)
	if days != 0 {
		t.Fatal("disabled instance collected usage")
	}
	csrf := c.csrf
	c.csrf = ""
	c.call("PATCH", "/usage", map[string]bool{"enabled": true}, 403)
	c.call("POST", "/usage/activity", map[string]any{}, 403)
	c.csrf = csrf
	c.call("POST", "/users", userInput{Username: "reader", Name: "Reader", Password: "Example-password-123!", Roles: []string{"3"}}, 201)
	reader := client(t, c.base)
	reader.login("reader", "Example-password-123!")
	reader.call("GET", "/usage", nil, 403)
	reader.call("PATCH", "/usage", map[string]bool{"enabled": true}, 403)
	reader.call("POST", "/usage/bind", map[string]string{"code": strings.Repeat("a", 64)}, 403)
	reader.call("POST", "/usage/unbind", map[string]any{}, 403)
	poll := decodeValue[map[string]any](t, reader.call("GET", "/poll?page=overview", nil, 200))
	if poll["usageEnabled"] != false || poll["usageConfirmationRequired"] != true {
		t.Fatal("poll must disclose pending confirmation to normal members")
	}
	reader.call("POST", "/usage/activity", map[string]any{}, 200)
}
func TestUsageAnonymousRegistrationActivityAndRestart(t *testing.T) {
	central, registered, reports, values := fakeUsageConsole(t)
	s, c, _ := usageFixture(t, central.URL)
	on := enableUsage(t, c)
	if !on.Enabled || len(on.InstanceID) != 32 || len(on.PendingActiveDays) != 0 {
		t.Fatalf("bad registration status %+v", on)
	}
	// Only an explicit activity event counts; neither registration nor polling does.
	c.call("GET", "/poll?page=overview", nil, 200)
	for range 3 {
		c.call("POST", "/usage/activity", map[string]any{}, 200)
	}
	_, err := s.Store.DB.Exec("INSERT INTO usage_activity(day) VALUES(?)", time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.usage.report(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registered.Load() != 1 || reports.Load() != 1 {
		t.Fatal("registration/report missing")
	}
	in, _ := values.Load("report")
	payload := in.(map[string]any)
	if len(payload) != 4 || len(payload["activeDays"].([]any)) != 1 {
		t.Fatalf("unexpected payload %#v", payload)
	}
	for _, forbidden := range []string{"hostname", "email", "username", "ip", "hostName", "targets", "files", "account", "arch", "deployment"} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("private field %s uploaded", forbidden)
		}
	}
	st := usageStateForTest(t, s)
	secret, _ := values.Load("credential")
	if st.Credential == secret.(string) || st.Credential == "" {
		t.Fatal("credential is not sealed")
	}
	if st.LastReportAt == 0 || st.NextReportAt-st.LastReportAt < 19440 || st.NextReportAt-st.LastReportAt > 23760 {
		t.Fatal("invalid report interval")
	}
	c.call("POST", "/usage/bind", map[string]string{"code": strings.Repeat("a", 64)}, 410)
	c.call("POST", "/usage/unbind", map[string]any{}, 410)
	raw := c.call("GET", "/usage", nil, 200)
	if strings.Contains(string(raw), secret.(string)) || strings.Contains(string(raw), "credential") {
		t.Fatal("secret leaked to status")
	}
	var audits string
	s.Store.DB.QueryRow("SELECT coalesce(group_concat(action||resource),'') FROM audit_logs").Scan(&audits)
	if strings.Contains(audits, secret.(string)) || strings.Contains(audits, strings.Repeat("a", 64)) {
		t.Fatal("secret leaked to audit")
	}
	rejectUsageDisable(t, c)
	if !usageStateForTest(t, s).Enabled {
		t.Fatal("required reporting was disabled")
	}
	dir := s.Store.Dir
	s.Store.Close()
	reopened, err := New(Config{DataDir: dir, ConsoleURL: central.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Store.Close()
	after, err := reopened.usage.status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !after.Enabled || after.ConfirmationRequired || after.InstanceID != on.InstanceID {
		t.Fatal("restart did not preserve required reporting and random identity")
	}
	// A new recipient cannot receive credentials or consent granted to the old one.
	reopened.Store.Close()
	changed, err := New(Config{DataDir: dir, ConsoleURL: "https://different.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer changed.Store.Close()
	after, err = changed.usage.status(context.Background())
	if err != nil || after.Enabled || after.InstanceID != "" {
		t.Fatalf("recipient change reused consent %+v %v", after, err)
	}
}
func TestUsageRequiredReportingDoesNotBlockMonitoringOrShutdown(t *testing.T) {
	started, cancelled := make(chan struct{}, 1), make(chan struct{}, 1)
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if strings.HasSuffix(r.URL.Path, "/register") {
			json.NewEncoder(w).Encode(usageResult{InstanceID: in["instanceId"].(string)})
			return
		}
		started <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer central.Close()
	s, c, _ := usageFixture(t, central.URL)
	enableUsage(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.usage.loop(ctx) }()
	defer func() { cancel(); <-done }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("report did not start")
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer local.Close()
	in := monitorPayload(local.URL)
	in.Threshold = 1
	id := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))["id"]
	job := decodeValue[map[string]string](t, c.call("POST", "/monitors/"+id+"/checks", map[string]any{}, 202))["jobId"]
	jobID, err := strconv.ParseInt(job, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.execute(context.Background(), jobID); err != nil {
		t.Fatal(err)
	}
	if alerts := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200)); len(alerts) != 1 {
		t.Fatal("stalled reporting blocked monitoring alerts")
	}
	rejectUsageDisable(t, c)
	if !s.usage.enabled.Load() || !usageStateForTest(t, s).Enabled {
		t.Fatal("rejected disable changed state")
	}
	select {
	case <-cancelled:
		t.Fatal("disable rejection cancelled report")
	default:
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel reporting")
	}
	if !usageStateForTest(t, s).Enabled {
		t.Fatal("shutdown changed durable reporting consent")
	}
}

func TestUsageConfirmationTransactionFailureAndRetry(t *testing.T) {
	central, registered, reports, _ := fakeUsageConsole(t)
	s, c, _ := usageFixture(t, central.URL)
	if _, err := s.Store.DB.Exec(`CREATE TRIGGER reject_confirmation BEFORE INSERT ON audit_logs WHEN NEW.action='Usage reporting enabled' BEGIN SELECT RAISE(ABORT,'test confirmation rollback'); END`); err != nil {
		t.Fatal(err)
	}
	c.call("PATCH", "/usage", usageConfirmation(t, c), 500)
	pending := usageStateForTest(t, s)
	if pending.Enabled || pending.Registered || s.usage.enabled.Load() || registered.Load() != 1 || reports.Load() != 0 {
		t.Fatal("failed audit partially enabled reporting")
	}
	var notice string
	var consent, activated int64
	if err := s.Store.DB.QueryRow("SELECT notice_version,consent_at,activated_at FROM usage_settings").Scan(&notice, &consent, &activated); err != nil {
		t.Fatal(err)
	}
	if notice != "" || consent != 0 || activated != 0 {
		t.Fatal("failed confirmation persisted consent")
	}
	if _, err := s.Store.DB.Exec("DROP TRIGGER reject_confirmation"); err != nil {
		t.Fatal(err)
	}
	on := enableUsage(t, c)
	if !on.Enabled || on.ConfirmationRequired || on.InstanceID != pending.InstanceID || usageStateForTest(t, s).Credential != pending.Credential {
		t.Fatal("confirmation retry changed identity or failed")
	}
	var audits int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action='Usage reporting enabled'").Scan(&audits); err != nil || audits != 1 {
		t.Fatal("confirmation audit not atomic")
	}
	s.Store.Close()
	next, err := New(Config{DataDir: s.Store.Dir, ConsoleURL: central.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Store.Close()
	if !next.usage.enabled.Load() || usageStateForTest(t, next).InstanceID != pending.InstanceID {
		t.Fatal("confirmed state not durable")
	}
	if err = next.Store.DB.QueryRow("SELECT notice_version FROM usage_settings").Scan(&notice); err != nil || notice != "2" {
		t.Fatal("v2 confirmation not preserved")
	}
}

func TestUsageTransportRejectsRedirectAndBacksOff(t *testing.T) {
	var destinationHits atomic.Int64
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationHits.Add(1) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/register") {
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			json.NewEncoder(w).Encode(usageResult{InstanceID: in["instanceId"].(string)})
			return
		}
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	s, c, _ := usageFixture(t, redirect.URL)
	enableUsage(t, c)
	if err := s.usage.report(context.Background()); err == nil {
		t.Fatal("redirect accepted")
	}
	if destinationHits.Load() != 0 {
		t.Fatal("credential could follow redirect")
	}
	st := usageStateForTest(t, s)
	if st.LastError != usageRejected || st.Failures != 1 || st.NextReportAt-time.Now().Unix() < 58 {
		t.Fatalf("missing backoff %+v", st)
	}
	if usageRetryDelay(30) > 21600 || usageRetryDelay(1) != 60 {
		t.Fatal("retry cap invalid")
	}
}
func TestUsageConfigurationAndMigration(t *testing.T) {
	for _, raw := range []string{"http://example.com", "http://localhost:8080", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=private", "https://example.com#fragment", "ftp://example.com"} {
		if _, err := normalizeConsoleURL(raw); err == nil {
			t.Errorf("accepted unsafe origin %s", raw)
		}
	}
	for _, raw := range []string{"https://console.example.com", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		if _, err := normalizeConsoleURL(raw); err != nil {
			t.Errorf("valid origin rejected %s: %v", raw, err)
		}
	}
	_, c, _ := usageFixture(t, "")
	c.call("PATCH", "/usage", map[string]bool{"enabled": true}, 422)
	dir, old := versionFourDatabase(t)
	if _, err := old.Exec(migration5 + migration6 + migration7 + migration8); err != nil {
		t.Fatal(err)
	}
	old.Close()
	migrated, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var version, enabled, count int
	migrated.DB.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version)
	migrated.DB.QueryRow("SELECT enabled FROM usage_settings WHERE id=1").Scan(&enabled)
	migrated.DB.QueryRow("SELECT count(*) FROM usage_activity").Scan(&count)
	if version != currentSchemaVersion || enabled != 0 || count != 0 {
		t.Fatal("migration enabled collection")
	}
	migrated.Close()
	// Failed schema migration rolls back and releases the actual SQLite file lock.
	failDir, db := versionFourDatabase(t)
	if _, err = db.Exec(migration5 + migration6 + migration7 + migration8 + `CREATE TRIGGER reject_nine BEFORE INSERT ON schema_migrations WHEN NEW.version=9 BEGIN SELECT RAISE(ABORT,'reject'); END`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if got, e := OpenStore(failDir); e == nil {
		got.Close()
		t.Fatal("migration should fail")
	}
	check, err := sql.Open("sqlite", filepath.Join(failDir, "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	check.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='usage_settings'").Scan(&count)
	if count != 0 {
		t.Fatal("failed migration partially committed")
	}
}
func TestUsageMissingMasterKeyDoesNotDiscardRegistration(t *testing.T) {
	central, _, _, _ := fakeUsageConsole(t)
	s, c, _ := usageFixture(t, central.URL)
	enableUsage(t, c)
	dir := s.Store.Dir
	s.Store.Close()
	if err := os.Remove(filepath.Join(dir, "master.key")); err != nil {
		t.Fatal(err)
	}
	if next, err := OpenStore(dir); err == nil {
		next.Close()
		t.Fatal("lost key silently replaced")
	}
}

func TestUsageEnabledRestartAndExpiredRegistrationRecovery(t *testing.T) {
	var registrations atomic.Int64
	var reject atomic.Int64
	var firstCredential string
	var mu sync.Mutex
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/register") {
			registrations.Add(1)
			mu.Lock()
			credential := in["credential"].(string)
			if firstCredential == "" {
				firstCredential = credential
			} else if credential != firstCredential {
				t.Error("registration changed credential")
			}
			mu.Unlock()
			reject.Store(0)
		} else if status := reject.Load(); status != 0 {
			w.WriteHeader(int(status))
			return
		}
		json.NewEncoder(w).Encode(usageResult{InstanceID: in["instanceId"].(string)})
	}))
	defer central.Close()
	s, c, _ := usageFixture(t, central.URL)
	enabled := enableUsage(t, c)
	if err := s.usage.report(context.Background()); err != nil {
		t.Fatal(err)
	}
	dir := s.Store.Dir
	s.Store.Close()
	next, err := New(Config{DataDir: dir, ConsoleURL: central.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Store.Close()
	state := usageStateForTest(t, next)
	if !state.Enabled || !state.Registered || state.InstanceID != enabled.InstanceID {
		t.Fatalf("restart lost identity/consent %+v", state)
	}
	if err = next.usage.report(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registrations.Load() != 1 {
		t.Fatal("restart registered a duplicate instance")
	}
	reject.Store(404)
	if err = next.usage.report(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registrations.Load() != 2 || !usageStateForTest(t, next).Registered {
		t.Fatal("expired registration was not recovered")
	}
	reject.Store(401)
	if err = next.usage.report(context.Background()); err == nil {
		t.Fatal("wrong credential was accepted")
	}
	if registrations.Load() != 2 {
		t.Fatal("wrong credential silently registered anew")
	}
}

func TestUsageActivityIsCoarseAndDoesNotWakeMonitoring(t *testing.T) {
	central, _, _, _ := fakeUsageConsole(t)
	s, c, _ := usageFixture(t, central.URL)
	enableUsage(t, c)
	for {
		select {
		case <-s.Store.checkWake:
			continue
		default:
		}
		break
	}
	for {
		select {
		case <-s.Store.notificationWake:
			continue
		default:
		}
		break
	}
	for range 3 {
		c.call("POST", "/usage/activity", map[string]any{}, 200)
	}
	var count int
	s.Store.DB.QueryRow("SELECT count(*) FROM usage_activity").Scan(&count)
	if count != 1 {
		t.Fatalf("activity should be one UTC day, got %d", count)
	}
	select {
	case <-s.Store.checkWake:
		t.Fatal("usage woke check scheduler")
	default:
	}
	select {
	case <-s.Store.notificationWake:
		t.Fatal("usage woke notification queue")
	default:
	}
	c.call("POST", "/usage/activity", map[string]string{"username": "should not be accepted"}, 422)
	rejectUsageDisable(t, c)
	s.Store.DB.QueryRow("SELECT count(*) FROM usage_activity").Scan(&count)
	if count != 1 {
		t.Fatal("rejected disable deleted pending activity")
	}
}

func TestUsageStatusOnlyIncludesAllowedStatistics(t *testing.T) {
	central, _, _, _ := fakeUsageConsole(t)
	_, c, _ := usageFixture(t, central.URL)
	enableUsage(t, c)
	status := decodeValue[map[string]any](t, c.call("GET", "/usage", nil, 200))
	if _, ok := status["account"]; ok {
		t.Fatal("account information remains in status")
	}
	preview := status["preview"].(map[string]any)
	if len(preview) != 4 || preview["instanceId"] == nil || preview["version"] == nil || preview["os"] == nil {
		t.Fatalf("extra anonymous preview fields: %#v", preview)
	}
	if _, ok := preview["activeDays"]; !ok {
		t.Fatal("missing UTC active-day preview")
	}
}

func TestUsageVersionDoesNotSendArbitraryBuildText(t *testing.T) {
	for _, version := range []string{"", "build from private-machine", "v1/branch", strings.Repeat("x", 101), "v1\r\nHeader: value"} {
		if got := usageVersion(version); got != "custom" {
			t.Fatalf("invalid version escaped: %q", got)
		}
	}
	if got := usageVersion("v0.4.0-dev+123-dirty"); got != "v0.4.0-dev+123-dirty" {
		t.Fatal(got)
	}
}
