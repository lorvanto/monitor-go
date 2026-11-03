package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUsageLegacyOffRequiresCurrentNoticeWithoutNetwork(t *testing.T) {
	central, registered, reports, _ := fakeUsageConsole(t)
	s, c, start := usageFixture(t, central.URL)
	start()
	rejectUsageDisable(t, c)
	for _, tc := range []struct {
		field string
		value any
	}{
		{"usageConsent", false}, {"usageNoticeVersion", "1"}, {"usageConsoleUrl", "https://previous.example.com"},
	} {
		in := usageConfirmation(t, c)
		in[tc.field] = tc.value
		c.call("PATCH", "/usage", in, 422)
	}
	c.call("PATCH", "/usage", map[string]any{"enabled": true}, 422)
	if registered.Load() != 0 || reports.Load() != 0 || usageStateForTest(t, s).Enabled || s.usage.enabled.Load() {
		t.Fatal("legacy off installation contacted Console without current consent")
	}
	_, missing, _ := usageFixture(t, "")
	missing.call("PATCH", "/usage", usageConfirmation(t, missing), 503)
}

func TestUsageLegacyConfirmationFailureRemainsOffAndRetriesSameIdentity(t *testing.T) {
	var reject atomic.Bool
	reject.Store(true)
	var requests atomic.Int64
	var mu sync.Mutex
	var id, credential string
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var in map[string]string
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if len(in) != 6 || r.URL.Path != "/api/v1/instances/register" {
			t.Error("confirmation transmitted unexpected fields or path")
		}
		mu.Lock()
		if id == "" {
			id, credential = in["instanceId"], in["credential"]
		} else if id != in["instanceId"] || credential != in["credential"] {
			t.Error("retry changed durable identity")
		}
		mu.Unlock()
		if reject.Load() {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(usageResult{InstanceID: in["instanceId"]})
	}))
	defer central.Close()
	s, c, _ := usageFixture(t, central.URL)
	c.call("PATCH", "/usage", usageConfirmation(t, c), 502)
	state := usageStateForTest(t, s)
	if state.Enabled || state.Registered || s.usage.enabled.Load() {
		t.Fatal("failed registration enabled reporting")
	}
	s.Store.Close()
	next, err := New(Config{DataDir: s.Store.Dir, ConsoleURL: central.URL, RequireActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Store.Close()
	local := httptest.NewServer(next.Handler())
	defer local.Close()
	nc := client(t, local.URL)
	nc.login("admin", "Example-password-123!")
	if next.usage.enabled.Load() || usageStateForTest(t, next).InstanceID != state.InstanceID {
		t.Fatal("restart lost pending state")
	}
	reject.Store(false)
	confirmed := enableUsage(t, nc)
	if !confirmed.Enabled || confirmed.ConfirmationRequired || confirmed.InstanceID != state.InstanceID || requests.Load() != 2 {
		t.Fatal("confirmation retry failed")
	}
	var notice string
	if err := next.Store.DB.QueryRow("SELECT notice_version FROM usage_settings").Scan(&notice); err != nil || notice != "2" {
		t.Fatal("new confirmation not persisted")
	}
}

func TestUsageEnabledUpgradePreservesHistoricalConsentAndBinding(t *testing.T) {
	central, registered, _, _ := fakeUsageConsole(t)
	s, c, _ := usageFixture(t, central.URL)
	enableUsage(t, c)
	c.call("POST", "/usage/bind", map[string]string{"code": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, 200)
	// Simulate the on-disk state of an installation activated under notice v1.
	if _, err := s.Store.DB.Exec("UPDATE usage_settings SET notice_version='1',consent_at=41,activated_at=42,last_report_at=43 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	before := usageStateForTest(t, s)
	s.Store.Close()
	next, err := New(Config{DataDir: s.Store.Dir, ConsoleURL: central.URL, RequireActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Store.Close()
	local := httptest.NewServer(next.Handler())
	defer local.Close()
	nc := client(t, local.URL)
	nc.login("admin", "Example-password-123!")
	status := decodeValue[usageStatus](t, nc.call("GET", "/usage", nil, 200))
	if !status.Enabled || !status.Required || status.ConfirmationRequired || status.NoticeVersion != "2" || status.Account == nil || status.Account.Login != "octocat" {
		t.Fatal("enabled upgrade lost reporting/account state")
	}
	// An idempotent request is not a new v2 confirmation, even with stale fields.
	nc.call("PATCH", "/usage", map[string]any{"enabled": true, "usageConsent": false, "usageNoticeVersion": "1", "usageConsoleUrl": "https://stale.example.com"}, 200)
	rejectUsageDisable(t, nc)
	if after := usageStateForTest(t, next); after != before {
		t.Fatal("upgrade/rejected disable changed durable usage state")
	}
	var version string
	var consent, activated int64
	if err := next.Store.DB.QueryRow("SELECT notice_version,consent_at,activated_at FROM usage_settings").Scan(&version, &consent, &activated); err != nil {
		t.Fatal(err)
	}
	if version != "1" || consent != 41 || activated != 42 || registered.Load() != 1 {
		t.Fatal("upgrade fabricated consent or registered twice")
	}
	poll := decodeValue[map[string]any](t, nc.call("GET", "/poll?page=overview", nil, 200))
	if poll["usageConfirmationRequired"] != false {
		t.Fatal("already enabled instance still prompted for confirmation")
	}
}

func TestUsageLegacyConfirmationConcurrentRequestsCommitOnce(t *testing.T) {
	central, registered, _, _ := fakeUsageConsole(t)
	s, c, _ := usageFixture(t, central.URL)
	input, _ := json.Marshal(usageConfirmation(t, c))
	var work sync.WaitGroup
	statuses := make(chan int, 3)
	for range 3 {
		work.Add(1)
		go func() {
			defer work.Done()
			req, _ := http.NewRequest("PATCH", c.base+"/api/v1/usage", bytes.NewReader(input))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-CSRF-Token", c.csrf)
			res, err := c.c.Do(req)
			if err != nil {
				statuses <- 0
				return
			}
			res.Body.Close()
			statuses <- res.StatusCode
		}()
	}
	work.Wait()
	close(statuses)
	for status := range statuses {
		if status != 200 {
			t.Fatalf("concurrent confirmation status %d", status)
		}
	}
	var audits int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action='Usage reporting enabled'").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if registered.Load() != 1 || audits != 1 {
		t.Fatal("concurrent confirmation duplicated registration/audit")
	}
}

func TestUsageLegacyConfirmationCancellationDoesNotEnable(t *testing.T) {
	started := make(chan struct{})
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer central.Close()
	s, c, _ := usageFixture(t, central.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, _ := json.Marshal(usageConfirmation(t, c))
	req, _ := http.NewRequestWithContext(ctx, "PATCH", c.base+"/api/v1/usage", bytes.NewReader(input))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	done := make(chan struct{})
	go func() {
		defer close(done)
		res, err := c.c.Do(req)
		if err == nil {
			res.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("confirmation did not start")
	}
	cancel()
	<-done
	// Synchronize with the handler's change lock before inspecting its result.
	s.usage.changeMu.Lock()
	s.usage.changeMu.Unlock()
	if usageStateForTest(t, s).Enabled || s.usage.enabled.Load() {
		t.Fatal("cancelled confirmation enabled reporting")
	}
}
