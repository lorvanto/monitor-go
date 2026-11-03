package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func activationFixture(t *testing.T, origin string, required bool) (*Server, *testClient) {
	t.Helper()
	s, err := New(Config{DataDir: t.TempDir(), Listen: "127.0.0.1:5080", ConsoleURL: origin, RequireActivation: required})
	if err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(s.Handler())
	t.Logf("activation HTTP fixture: %s", local.URL)
	t.Cleanup(func() { local.Close(); s.Store.Close() })
	return s, client(t, local.URL)
}

func activationInput(s *Server) map[string]any {
	return map[string]any{"token": s.setupToken, "username": "admin", "name": "Private administrator name", "password": "Private-admin-password-123!", "usageConsent": true, "usageNoticeVersion": "1", "usageConsoleUrl": s.usage.origin}
}

func activationCounts(t *testing.T, s *Server) (users, enabled, registered int) {
	t.Helper()
	if err := s.Store.DB.QueryRow("SELECT (SELECT count(*) FROM users),enabled,registered FROM usage_settings WHERE id=1").Scan(&users, &enabled, &registered); err != nil {
		t.Fatal(err)
	}
	return
}

func assertActivationPending(t *testing.T, s *Server) {
	t.Helper()
	users, enabled, registered := activationCounts(t, s)
	if users != 0 || enabled != 0 || registered != 0 || s.usage.enabled.Load() {
		t.Fatalf("failed setup created administrator or enabled reporting: users=%d enabled=%d registered=%d", users, enabled, registered)
	}
	var notice string
	var consentAt, activatedAt int64
	if err := s.Store.DB.QueryRow("SELECT notice_version,consent_at,activated_at FROM usage_settings").Scan(&notice, &consentAt, &activatedAt); err != nil {
		t.Fatal(err)
	}
	if notice != "" || consentAt != 0 || activatedAt != 0 {
		t.Fatal("failed setup persisted completed activation/consent")
	}
}

func TestActivationConsentAndValidationMakeNoNetworkRequest(t *testing.T) {
	central, registered, reports, _ := fakeUsageConsole(t)
	s, c := activationFixture(t, central.URL, true)
	status := decodeValue[struct {
		Initialized bool             `json:"initialized"`
		Activation  activationStatus `json:"activation"`
	}](t, c.call("GET", "/status", nil, 200))
	if status.Initialized || !status.Activation.Required || !status.Activation.Configured || status.Activation.ConsoleURL != central.URL || status.Activation.NoticeVersion != "1" {
		t.Fatalf("incorrect initial activation status: %+v", status)
	}
	for _, tc := range []struct {
		field  string
		value  any
		status int
	}{
		{"usageConsent", false, 422}, {"usageNoticeVersion", "0", 422}, {"usageConsoleUrl", "https://previous.example.com", 422}, {"token", "wrong", 403}, {"username", "?", 422}, {"password", "short", 422}, {"name", "", 422},
	} {
		in := activationInput(s)
		in[tc.field] = tc.value
		c.call("POST", "/setup", in, tc.status)
	}
	in := activationInput(s)
	delete(in, "usageConsent")
	c.call("POST", "/setup", in, 422)
	raw, _ := json.Marshal(activationInput(s))
	req, _ := http.NewRequest("POST", c.base+"/api/v1/setup", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://untrusted.example")
	res, err := c.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("cross-origin setup status %d", res.StatusCode)
	}
	if registered.Load() != 0 || reports.Load() != 0 {
		t.Fatal("invalid/unconsented setup contacted Console")
	}
	assertActivationPending(t, s)
	if usageStateForTest(t, s).InstanceID != "" {
		t.Fatal("invalid setup generated an identity")
	}
}

func TestActivationMissingDestinationAndExplicitOfflineMode(t *testing.T) {
	s, c := activationFixture(t, "", true)
	c.call("POST", "/setup", activationInput(s), 503)
	assertActivationPending(t, s)
	central, registered, reports, _ := fakeUsageConsole(t)
	offline, oc := activationFixture(t, central.URL, false)
	in := activationInput(offline)
	delete(in, "usageConsent")
	delete(in, "usageNoticeVersion")
	oc.call("POST", "/setup", in, 200)
	users, enabled, _ := activationCounts(t, offline)
	if users != 1 || enabled != 0 || registered.Load() != 0 || reports.Load() != 0 {
		t.Fatal("explicit offline setup required registration or enabled reporting")
	}
}

func TestActivationRetryAfterRemoteFailureUsesSameIdentityAndNoSecrets(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	var requests atomic.Int64
	var identity, credential string
	var mu sync.Mutex
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if r.URL.Path != "/api/v1/instances/register" || len(in) != 6 {
			t.Error("activation sent unexpected fields or endpoint")
		}
		for _, key := range []string{"token", "username", "name", "password", "activeDays"} {
			if _, ok := in[key]; ok {
				t.Errorf("activation leaked %s", key)
			}
		}
		id, _ := in["instanceId"].(string)
		secret, _ := in["credential"].(string)
		mu.Lock()
		if identity == "" {
			identity, credential = id, secret
		} else if identity != id || credential != secret {
			t.Error("retry changed installation identity")
		}
		mu.Unlock()
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(usageResult{InstanceID: id})
	}))
	defer central.Close()
	s, c := activationFixture(t, central.URL, true)
	c.call("POST", "/setup", activationInput(s), 502)
	assertActivationPending(t, s)
	state := usageStateForTest(t, s)
	mu.Lock()
	storedPlaintext := credential
	mu.Unlock()
	if len(state.InstanceID) != 32 || state.Credential == storedPlaintext {
		t.Fatal("pending identity missing or credential not encrypted")
	}
	// The prepared identity survives process restart, including a lost remote reply.
	s.Store.Close()
	next, err := New(Config{DataDir: s.Store.Dir, ConsoleURL: central.URL, RequireActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Store.Close()
	local := httptest.NewServer(next.Handler())
	defer local.Close()
	fail.Store(false)
	nc := client(t, local.URL)
	nc.call("POST", "/setup", activationInput(next), 200)
	if requests.Load() != 2 {
		t.Fatalf("expected one retry, got %d requests", requests.Load())
	}
	users, enabled, registered := activationCounts(t, next)
	if users != 1 || enabled != 1 || registered != 1 {
		t.Fatal("successful activation not committed")
	}
	var version string
	var consentAt, activatedAt int64
	if err = next.Store.DB.QueryRow("SELECT notice_version,consent_at,activated_at FROM usage_settings").Scan(&version, &consentAt, &activatedAt); err != nil {
		t.Fatal(err)
	}
	if version != "1" || consentAt == 0 || activatedAt != consentAt {
		t.Fatal("activation/consent record missing")
	}
}

func TestActivationUserTransactionFailureDoesNotStartReports(t *testing.T) {
	central, registered, reports, _ := fakeUsageConsole(t)
	s, c := activationFixture(t, central.URL, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.usage.loop(ctx) }()
	defer func() { cancel(); <-done }()
	// Reject the final audit write, after both the user and activation state have
	// been written in the transaction, to verify that all of them roll back.
	if _, err := s.Store.DB.Exec("CREATE TRIGGER reject_setup BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT,'test rollback'); END"); err != nil {
		t.Fatal(err)
	}
	c.call("POST", "/setup", activationInput(s), 500)
	assertActivationPending(t, s)
	firstID := usageStateForTest(t, s).InstanceID
	if registered.Load() != 1 || reports.Load() != 0 {
		t.Fatal("reporting started before administrator transaction committed")
	}
	if _, err := s.Store.DB.Exec("DROP TRIGGER reject_setup"); err != nil {
		t.Fatal(err)
	}
	c.call("POST", "/setup", activationInput(s), 200)
	if usageStateForTest(t, s).InstanceID != firstID {
		t.Fatal("transaction retry created a duplicate installation")
	}
	waitUsage(t, func() bool { return reports.Load() == 1 })
}

func TestActivationRejectsRedirectAndInvalidRegistrationResponses(t *testing.T) {
	var redirected atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer other.Close()
	for _, scenario := range []string{"redirect", "wrong identity", "invalid JSON"} {
		t.Run(scenario, func(t *testing.T) {
			central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				switch scenario {
				case "redirect":
					http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
				case "wrong identity":
					json.NewEncoder(w).Encode(usageResult{InstanceID: strings.Repeat("a", 32)})
				case "invalid JSON":
					w.Write([]byte("not JSON"))
				}
			}))
			defer central.Close()
			s, c := activationFixture(t, central.URL, true)
			c.call("POST", "/setup", activationInput(s), 502)
			assertActivationPending(t, s)
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("activation followed a redirect and sent credentials to another origin")
	}
}

func TestActivationConcurrentSetupIsSingleRegistrationAndAdministrator(t *testing.T) {
	central, registered, _, _ := fakeUsageConsole(t)
	s, c := activationFixture(t, central.URL, true)
	input, _ := json.Marshal(activationInput(s))
	codes := make(chan int, 4)
	var work sync.WaitGroup
	for range 4 {
		work.Add(1)
		go func() {
			defer work.Done()
			req, _ := http.NewRequest("POST", c.base+"/api/v1/setup", bytes.NewReader(input))
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				codes <- 0
				return
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			codes <- res.StatusCode
		}()
	}
	work.Wait()
	close(codes)
	success, conflict := 0, 0
	for status := range codes {
		if status == 200 {
			success++
		} else if status == 409 {
			conflict++
		} else {
			t.Errorf("unexpected setup response %d", status)
		}
	}
	users, _, _ := activationCounts(t, s)
	if success != 1 || conflict != 3 || registered.Load() != 1 || users != 1 {
		t.Fatalf("setup was not single-use: successes=%d conflicts=%d registrations=%d users=%d", success, conflict, registered.Load(), users)
	}
}

func TestActivationNetworkDoesNotHoldWriterAndTimesOut(t *testing.T) {
	started := make(chan struct{}, 1)
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer central.Close()
	s, c := activationFixture(t, central.URL, true)
	s.usage.client.Timeout = 150 * time.Millisecond
	finished := make(chan int, 1)
	in, _ := json.Marshal(activationInput(s))
	go func() {
		res, err := http.Post(c.base+"/api/v1/setup", "application/json", bytes.NewReader(in))
		if err != nil {
			finished <- 0
			return
		}
		res.Body.Close()
		finished <- res.StatusCode
	}()
	select {
	case <-started:
	case <-time.After(45 * time.Second):
		t.Fatal("activation request did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := s.Store.DB.ExecContext(ctx, "UPDATE system_settings SET instance_name=instance_name WHERE id=1"); err != nil {
		t.Fatalf("remote request held writer: %v", err)
	}
	select {
	case status := <-finished:
		if status != 502 {
			t.Fatalf("unexpected timeout status %d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("activation timeout not bounded")
	}
	assertActivationPending(t, s)
}

func TestActivatedInstallationRestartsAndRunsAlertsWithoutConsole(t *testing.T) {
	central, _, _, _ := fakeUsageConsole(t)
	s, c := activationFixture(t, central.URL, true)
	c.call("POST", "/setup", activationInput(s), 200)
	id := usageStateForTest(t, s).InstanceID
	s.Store.Close()
	central.Close()
	next, err := New(Config{DataDir: s.Store.Dir, ConsoleURL: central.URL, RequireActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Store.Close()
	local := httptest.NewServer(next.Handler())
	defer local.Close()
	nc := client(t, local.URL)
	status := decodeValue[struct {
		Initialized bool             `json:"initialized"`
		Activation  activationStatus `json:"activation"`
	}](t, nc.call("GET", "/status", nil, 200))
	if !status.Initialized || status.Activation.Required || usageStateForTest(t, next).InstanceID != id {
		t.Fatal("activated restart requires activation again or loses identity")
	}
	nc.login("admin", "Private-admin-password-123!")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); next.usage.loop(ctx) }()
	defer func() { cancel(); <-done }()
	waitUsage(t, func() bool { return usageStateForTest(t, next).Failures > 0 })
	if toggleUsage(t, nc, false).Enabled {
		t.Fatal("activation prevents opt-out")
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer broken.Close()
	in := monitorPayload(broken.URL)
	in.Threshold = 1
	monitorID := decodeValue[map[string]string](t, nc.call("POST", "/monitors", in, 201))["id"]
	job := decodeValue[map[string]string](t, nc.call("POST", "/monitors/"+monitorID+"/checks", map[string]any{}, 202))["jobId"]
	jobID, _ := strconv.ParseInt(job, 10, 64)
	if err := next.execute(context.Background(), jobID); err != nil {
		t.Fatal(err)
	}
	if alerts := decodeValue[[]Alert](t, nc.call("GET", "/alerts", nil, 200)); len(alerts) != 1 {
		t.Fatal("Console outage/opt-out interrupted monitoring alerts")
	}
	cancel()
	<-done
	next.Store.Close()
	restarted, err := New(Config{DataDir: s.Store.Dir, ConsoleURL: central.URL, RequireActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Store.Close()
	if restarted.usage.enabled.Load() || usageStateForTest(t, restarted).InstanceID != id {
		t.Fatal("restart lost opt-out or activated identity")
	}
}

func TestExistingInstallationDoesNotRequireActivationOnUpgrade(t *testing.T) {
	central, registered, reports, _ := fakeUsageConsole(t)
	s, c := activationFixture(t, central.URL, false)
	c.call("POST", "/setup", activationInput(s), 200)
	s.Store.Close()
	next, err := New(Config{DataDir: s.Store.Dir, ConsoleURL: central.URL, RequireActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Store.Close()
	local := httptest.NewServer(next.Handler())
	defer local.Close()
	nc := client(t, local.URL)
	nc.login("admin", "Private-admin-password-123!")
	status := decodeValue[struct {
		Activation activationStatus `json:"activation"`
	}](t, nc.call("GET", "/status", nil, 200))
	if status.Activation.Required || next.usage.enabled.Load() || registered.Load() != 0 || reports.Load() != 0 {
		t.Fatal("upgrade silently required or enabled activation")
	}
}

func TestActivationMigrationPreservesUsageAndRollsBackOnFailure(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(strconv.FormatBool(reject), func(t *testing.T) {
			dir, old := versionFourDatabase(t)
			if _, err := old.Exec(migration5 + migration6 + migration7 + migration8 + migration9); err != nil {
				t.Fatal(err)
			}
			if _, err := old.Exec("UPDATE usage_settings SET enabled=1,console_url='https://console.example.com',instance_id=?,registered=1,last_report_at=42,account_login='owner'", strings.Repeat("a", 32)); err != nil {
				t.Fatal(err)
			}
			if reject {
				if _, err := old.Exec("CREATE TRIGGER reject_ten BEFORE INSERT ON schema_migrations WHEN NEW.version=10 BEGIN SELECT RAISE(ABORT,'test migration rollback'); END"); err != nil {
					t.Fatal(err)
				}
			}
			old.Close()
			store, err := OpenStore(dir)
			if !reject {
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				st, err := readUsageState(context.Background(), store.DB)
				if err != nil || !st.Enabled || !st.Registered || st.InstanceID != strings.Repeat("a", 32) || st.LastReportAt != 42 || st.AccountLogin != "owner" {
					t.Fatal("migration changed prior reporting choice or registration")
				}
				var notice string
				var consent, activated int64
				if err := store.DB.QueryRow("SELECT notice_version,consent_at,activated_at FROM usage_settings").Scan(&notice, &consent, &activated); err != nil {
					t.Fatal(err)
				}
				if notice != "" || consent != 0 || activated != 0 {
					t.Fatal("migration fabricated consent")
				}
				return
			}
			if err == nil {
				store.Close()
				t.Fatal("expected migration rejection")
			}
			check, err := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer check.Close()
			var version, columns int
			check.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version)
			check.QueryRow("SELECT count(*) FROM pragma_table_info('usage_settings') WHERE name IN ('notice_version','consent_at','activated_at')").Scan(&columns)
			if version != 9 || columns != 0 {
				t.Fatal("failed migration partially committed")
			}
		})
	}
}
