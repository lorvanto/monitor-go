package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	agentclient "github.com/lorvanto/monitor-go/internal/agent"
	"github.com/lorvanto/monitor-go/internal/hoststats"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustSQL(t *testing.T, s *Server, q string, args ...any) {
	t.Helper()
	if _, e := s.Store.DB.Exec(q, args...); e != nil {
		t.Fatal(e)
	}
}
func agentRequest(t *testing.T, base, path, token string, body any, want int) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+"/api/v1/agent/"+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("agent %s got %d want %d: %s", path, res.StatusCode, want, raw)
	}
}
func TestAgentLifecycleIsolationAndOffline(t *testing.T) {
	s, ts, c := fixture(t)
	v := decodeValue[struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}](t, c.call("POST", "/hosts/agents", map[string]string{"alias": "远程测试机"}, 200))
	if v.ID != "2" {
		t.Fatal("local host ID must remain reserved")
	}
	credential := randomToken()
	enroll := map[string]string{"token": v.Token, "credential": credential}
	agentRequest(t, ts.URL, "enroll", "", enroll, 200)
	agentRequest(t, ts.URL, "enroll", "", enroll, 200)
	agentRequest(t, ts.URL, "enroll", "", map[string]string{"token": v.Token, "credential": randomToken()}, 409)
	value := 22.5
	report := hoststats.Report{Protocol: 1, ReportID: randomToken(), AgentVersion: "0.2.0", CollectedAt: now(), Hostname: "remote.test", Inventory: inventory{OS: "Linux", IPs: []string{"10.0.0.2/24"}}, Snapshot: snapshot{CPU: &value, Memory: &value}}
	agentRequest(t, ts.URL, "reports", credential, report, 200)
	agentRequest(t, ts.URL, "reports", credential, report, 200)
	var n int
	s.Store.DB.QueryRow("SELECT count(*) FROM host_samples WHERE host_id=2").Scan(&n)
	if n != 1 {
		t.Fatal("retry duplicated sample")
	}
	agentRequest(t, ts.URL, "reports", randomToken(), report, 401)
	hosts := decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if hosts[0].Status != "online" || hosts[0].Hostname != "remote.test" {
		t.Fatal("remote host missing")
	}
	raw := string(c.call("GET", "/hosts", nil, 200))
	if strings.Contains(raw, credential) || strings.Contains(raw, v.Token) {
		t.Fatal("host API leaks credential")
	}
	mustSQL(t, s, "UPDATE hosts SET last_seen=?,enrolled_at=? WHERE id=2", now()-100000, now()-100000)
	if e := s.detectOfflineAgents(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := s.detectOfflineAgents(context.Background()); e != nil {
		t.Fatal(e)
	}
	alerts := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	if len(alerts) != 1 || alerts[0].State != "open" {
		t.Fatal("offline event missing or duplicated")
	}
	report.ReportID = randomToken()
	report.CollectedAt += 10
	agentRequest(t, ts.URL, "reports", credential, report, 200)
	alerts = decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	if alerts[0].State != "resolved" {
		t.Fatal("offline recovery missing")
	}
	report.ReportID = randomToken()
	report.CollectedAt--
	agentRequest(t, ts.URL, "reports", credential, report, 409)
	report.CollectedAt = now() - 600000
	agentRequest(t, ts.URL, "reports", credential, report, 422)
	c.call("POST", "/hosts/2/agent", map[string]any{"version": hosts[0].Version, "action": "revoke"}, 200)
	report.CollectedAt = now()
	report.ReportID = randomToken()
	agentRequest(t, ts.URL, "reports", credential, report, 401)
	agentRequest(t, ts.URL, "enroll", "", enroll, 401)
	hosts = decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if hosts[0].Status != "revoked" {
		t.Fatal("revoked state missing")
	}
	replacement := decodeValue[map[string]any](t, c.call("POST", "/hosts/2/agent", map[string]any{"version": hosts[0].Version, "action": "reenroll"}, 200))
	mustSQL(t, s, "UPDATE hosts SET enrollment_expires=? WHERE id=2", now()-1)
	agentRequest(t, ts.URL, "enroll", "", map[string]any{"token": replacement["token"], "credential": randomToken()}, 401)
	c.call("POST", "/users", userInput{Username: "agentreader", Name: "只读用户", Password: "Readonly-test-123!", Roles: []string{"3"}}, 201)
	reader := client(t, ts.URL)
	reader.login("agentreader", "Readonly-test-123!")
	reader.call("POST", "/hosts/agents", map[string]string{"alias": "forbidden"}, 403)
	reader.call("GET", "/notification-channels", nil, 403)
	reader.call("POST", "/notification-deliveries/1/retry", nil, 403)
}
func TestActualAgentClientAndConfig(t *testing.T) {
	s, ts, c := fixture(t)
	v := decodeValue[map[string]any](t, c.call("POST", "/hosts/agents", map[string]string{"alias": "Agent 客户端"}, 200))
	dir := t.TempDir()
	file := filepath.Join(dir, "enrollment-token")
	os.WriteFile(file, []byte(v["token"].(string)), 0600)
	config := filepath.Join(dir, "agent", "config.json")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- agentclient.Run(ctx, config, ts.URL, file, "") }()
	defer func() {
		cancel()
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		s.Store.DB.QueryRow("SELECT count(*) FROM host_samples WHERE host_id=2").Scan(&count)
		if count > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	var count int
	s.Store.DB.QueryRow("SELECT count(*) FROM host_samples WHERE host_id=2").Scan(&count)
	if count != 1 {
		t.Fatal("Agent client did not report real sample")
	}
	info, e := os.Stat(config)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("Agent credential permissions")
	}
	raw, _ := os.ReadFile(config)
	if strings.Contains(string(raw), v["token"].(string)) {
		t.Fatal("enrollment code retained after enrollment")
	}
	loaded, e := agentclient.LoadOrCreate(config, "", "", "")
	if e != nil || loaded.EnrollmentToken != "" {
		t.Fatal("cannot restart saved agent")
	}
	if e = agentclient.ValidateServer("http://192.0.2.1"); e == nil {
		t.Fatal("remote plaintext server allowed")
	}
}

func TestAgentRegisteredWithoutFirstSampleBecomesOffline(t *testing.T) {
	s, ts, c := fixture(t)
	v := decodeValue[map[string]any](t, c.call("POST", "/hosts/agents", map[string]string{"alias": "尚未采样"}, 200))
	agentRequest(t, ts.URL, "enroll", "", map[string]any{"token": v["token"], "credential": randomToken()}, 200)
	hosts := decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if hosts[0].Status != "pending" {
		t.Fatal("fresh enrollment should await first sample")
	}
	mustSQL(t, s, "UPDATE hosts SET enrolled_at=? WHERE id=2", now()-100000)
	if e := s.detectOfflineAgents(context.Background()); e != nil {
		t.Fatal(e)
	}
	hosts = decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if hosts[0].Status != "offline" {
		t.Fatal("registered but silent Agent must be offline")
	}
	alerts := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	if len(alerts) != 1 || alerts[0].State != "open" {
		t.Fatal("first-report timeout missing")
	}
}
func makeChannel(t *testing.T, c *testClient, target string) string {
	t.Helper()
	v := decodeValue[map[string]string](t, c.call("POST", "/notification-channels", channelInput{Name: "Webhook 验收", Kind: "webhook", Enabled: true, Config: channelConfig{URL: target, Secret: "test-signing-secret"}}, 200))
	return v["id"]
}
func runDelivery(t *testing.T, s *Server, id int64) {
	t.Helper()
	mustSQL(t, s, "UPDATE notification_deliveries SET state='sending' WHERE id=?", id)
	if e := s.deliver(context.Background(), id); e != nil {
		t.Fatal(e)
	}
}
func TestNotificationRetrySilenceRecoveryAndRestart(t *testing.T) {
	s, _, c := fixture(t)
	var requests atomic.Int64
	var fail atomic.Bool
	fail.Store(true)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte("test-signing-secret"))
		mac.Write([]byte(r.Header.Get("X-MonitorGo-Timestamp") + "."))
		mac.Write(body)
		if r.Header.Get("X-MonitorGo-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid webhook signature")
		}
		var p notificationPayload
		json.Unmarshal(body, &p)
		if p.DeliveryID == "" || p.DeliveryID != r.Header.Get("X-MonitorGo-Delivery-ID") {
			t.Error("missing stable delivery ID")
		}
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer receiver.Close()
	channel := makeChannel(t, c, receiver.URL)
	raw := string(c.call("GET", "/notification-channels", nil, 200))
	if strings.Contains(raw, "test-signing-secret") {
		t.Fatal("secret exposed")
	}
	var encrypted string
	s.Store.DB.QueryRow("SELECT config FROM notification_channels").Scan(&encrypted)
	if strings.Contains(encrypted, receiver.URL) || strings.Contains(encrypted, "test-signing-secret") {
		t.Fatal("channel config not encrypted")
	}
	transition := func(active bool) {
		t.Helper()
		if e := s.Store.write(context.Background(), func(tx *sql.Tx) error {
			return transitionAlert(tx, "test:notify", "monitor", 7, "web.test", "服务异常", "响应超时", "critical", active)
		}); e != nil {
			t.Fatal(e)
		}
	}
	transition(true)
	transition(true)
	var count int
	s.Store.DB.QueryRow("SELECT count(*) FROM notification_deliveries").Scan(&count)
	if count != 1 {
		t.Fatal("notifications not deduplicated")
	}
	runDelivery(t, s, 1)
	var state string
	var attempts int
	var next int64
	s.Store.DB.QueryRow("SELECT state,attempts,next_attempt FROM notification_deliveries WHERE id=1").Scan(&state, &attempts, &next)
	if state != "queued" || attempts != 1 || next <= now()+10000 {
		t.Fatal("retry/backoff not persisted")
	}
	// Emulate process termination after claiming a persisted task.
	mustSQL(t, s, "UPDATE notification_deliveries SET state='sending' WHERE id=1")
	dir := s.Store.Dir
	s.Store.Close()
	reopened, e := OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	s.Store = reopened
	s.Store.DB.QueryRow("SELECT state FROM notification_deliveries WHERE id=1").Scan(&state)
	if state != "queued" {
		t.Fatal("crash task not recovered")
	}
	fail.Store(false)
	runDelivery(t, s, 1)
	s.Store.DB.QueryRow("SELECT state,attempts FROM notification_deliveries WHERE id=1").Scan(&state, &attempts)
	if state != "sent" || attempts != 2 {
		t.Fatal("retry not sent")
	}
	transition(false)
	runDelivery(t, s, 2)
	transition(true)
	alerts := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	var active Alert
	for _, a := range alerts {
		if a.State == "open" {
			active = a
		}
	}
	c.call("POST", "/alerts/"+idText(active.ID)+"/silences", map[string]int{"version": active.Version, "minutes": 30}, 200)
	runDelivery(t, s, 3)
	transition(false)
	runDelivery(t, s, 4)
	s.Store.DB.QueryRow("SELECT count(*) FROM notification_deliveries WHERE state='suppressed'").Scan(&count)
	if count != 2 || requests.Load() != 3 {
		t.Fatal("silence failed")
	}
	// A stale opening must not arrive after recovery, but recovery itself can.
	transition(true)
	transition(false)
	runDelivery(t, s, 5)
	runDelivery(t, s, 6)
	s.Store.DB.QueryRow("SELECT state FROM notification_deliveries WHERE id=5").Scan(&state)
	if state != "suppressed" {
		t.Fatal("stale opening was sent")
	}
	job := decodeValue[map[string]string](t, c.call("POST", "/notification-channels/"+channel+"/tests", map[string]int{"version": 1}, 200))
	if job["deliveryId"] != "7" {
		t.Fatal("missing test queue")
	}
	fail.Store(true)
	for i := 0; i < 4; i++ {
		runDelivery(t, s, 7)
	}
	s.Store.DB.QueryRow("SELECT state FROM notification_deliveries WHERE id=7").Scan(&state)
	if state != "failed" {
		t.Fatal("retry limit missing")
	}
	c.call("POST", "/notification-deliveries/7/retry", map[string]any{}, 200)
	fail.Store(false)
	runDelivery(t, s, 7)
	c.call("PATCH", "/notification-channels/"+channel, channelInput{Name: "changed", Kind: "webhook", Version: 99, Enabled: true, Config: channelConfig{URL: receiver.URL}}, 409)
	c.call("PATCH", "/notification-channels/"+channel, channelInput{Name: "changed", Kind: "webhook", Version: 1, Enabled: false, Config: channelConfig{URL: receiver.URL}}, 200)
	channels := decodeValue[[]notificationChannel](t, c.call("GET", "/notification-channels", nil, 200))
	if !channels[0].HasSecret {
		t.Fatal("blank edit lost secret")
	}
	c.call("POST", "/notification-channels/"+channel+"/tests", map[string]int{"version": 2}, 422)
	c.call("DELETE", "/notification-channels/"+channel, map[string]int{"version": 2}, 200)
	ds := decodeValue[[]deliveryView](t, c.call("GET", "/notification-deliveries", nil, 200))
	if len(ds) != 7 {
		t.Fatal("delivery records missing")
	}
}
func TestV1UpgradePreservesDataAndGrantsOnlyAdmin(t *testing.T) {
	dir := t.TempDir()
	db, e := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schema); e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO roles(id,name,builtin) VALUES(1,'超级管理员',1),(2,'运维工程师',1); INSERT INTO role_permissions VALUES(1,'role:manage'),(2,'host:read'); INSERT INTO users(id,username,display_name,password_hash) VALUES(1,'legacy','旧用户','hash'); INSERT INTO user_roles VALUES(1,1); INSERT INTO sessions VALUES('old',1,'csrf',1,9999999999999); INSERT INTO hosts(id,hostname,alias) VALUES(1,'legacy-host','原有主机');`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var name string
	var count int
	s.DB.QueryRow("SELECT alias FROM hosts WHERE id=1").Scan(&name)
	if name != "原有主机" {
		t.Fatal("migration lost data")
	}
	s.DB.QueryRow("SELECT count(*) FROM role_permissions WHERE role_id=1 AND permission IN ('host:agents','notification:manage')").Scan(&count)
	if count != 2 {
		t.Fatal("admin not upgraded")
	}
	s.DB.QueryRow("SELECT count(*) FROM role_permissions WHERE role_id=2 AND permission IN ('host:agents','notification:manage')").Scan(&count)
	if count != 0 {
		t.Fatal("unexpected escalation")
	}
	s.DB.QueryRow("SELECT count(*) FROM sessions").Scan(&count)
	if count != 0 {
		t.Fatal("old admin session not revoked")
	}
}
