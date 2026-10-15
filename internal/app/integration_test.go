package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

type testClient struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
}

func (c *testClient) call(method, path string, body any, want int) json.RawMessage {
	c.t.Helper()
	b, _ := json.Marshal(body)
	req, e := http.NewRequest(method, c.base+"/api/v1"+path, bytes.NewReader(b))
	if e != nil {
		c.t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	res, e := c.c.Do(req)
	if e != nil {
		c.t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: got %d want %d body %s", method, path, res.StatusCode, want, raw)
	}
	var v struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(raw, &v)
	return v.Data
}
func client(t *testing.T, base string) *testClient {
	jar, _ := cookiejar.New(nil)
	return &testClient{t: t, base: base, c: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}
func (c *testClient) login(username, password string) {
	c.call("POST", "/auth/login", map[string]string{"username": username, "password": password}, 200)
	var u User
	json.Unmarshal(c.call("GET", "/me", nil, 200), &u)
	c.csrf = u.CSRF
}
func fixture(t *testing.T) (*Server, *httptest.Server, *testClient) {
	t.Helper()
	cfg := Config{DataDir: t.TempDir(), Listen: "127.0.0.1:5080", Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("app")}}}
	s, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Store.Close() })
	c := client(t, ts.URL)
	c.call("POST", "/setup", map[string]string{"token": s.setupToken, "username": "admin", "name": "测试管理员", "password": "Test-password-123!"}, 200)
	var u User
	json.Unmarshal(c.call("GET", "/me", nil, 200), &u)
	c.csrf = u.CSRF
	return s, ts, c
}
func decodeValue[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if e := json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func TestAuthenticationAuthorizationAndConflicts(t *testing.T) {
	s, ts, c := fixture(t)
	if _, e := os.Stat(filepath.Join(s.Store.Dir, "setup-token")); !os.IsNotExist(e) {
		t.Fatal("setup token must be removed")
	}
	unauth := client(t, ts.URL)
	unauth.call("GET", "/monitors", nil, 401)
	unauth.call("POST", "/auth/login", map[string]string{"username": "admin", "password": "incorrect"}, 401)
	csrf := c.csrf
	c.csrf = ""
	c.call("POST", "/roles", map[string]string{"name": "blocked"}, 403)
	c.csrf = csrf
	created := decodeValue[map[string]string](t, c.call("POST", "/users", userInput{Username: "reader", Name: "只读用户", Password: "Read-password-123!", Roles: []string{"3"}}, 201))
	reader := client(t, ts.URL)
	reader.login("reader", "Read-password-123!")
	reader.call("GET", "/monitors", nil, 200)
	reader.call("POST", "/monitors", map[string]string{}, 403)
	reader.call("GET", "/roles", nil, 403)
	c.call("PATCH", "/roles/3", map[string]any{"version": 99, "permissions": []string{"host:read"}}, 409)
	c.call("PATCH", "/roles/3", map[string]any{"version": 1, "permissions": []string{"host:read"}}, 200)
	reader.call("GET", "/me", nil, 401)
	reader.login("reader", "Read-password-123!")
	reader.call("GET", "/monitors", nil, 403)
	c.call("PATCH", "/users/1", userInput{Name: "管理员", Enabled: false, Roles: []string{"1"}, Version: 1}, 422)
	c.call("GET", "/me", nil, 200)
	c.call("PATCH", "/roles/1", map[string]any{"version": 1, "permissions": []string{}}, 422)
	c.call("PATCH", "/users/"+created["id"], userInput{Name: "只读用户", Enabled: false, Roles: []string{"3"}, Version: 1}, 200)
	reader.call("GET", "/me", nil, 401)
	c.call("POST", "/auth/logout", map[string]any{}, 200)
	c.call("GET", "/me", nil, 401)
	var n int
	s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs").Scan(&n)
	if n < 4 {
		t.Fatal("missing audit records")
	}
}
func monitorPayload(target string) monitorInput {
	return monitorInput{Name: "本地验收检测", Type: "HTTP", Target: target, Group: "测试", Interval: 60, Timeout: 3, Expected: 200, Threshold: 2, SSLDays: 14}
}
func TestMonitorAlertLifecycleAndRestart(t *testing.T) {
	s, _, c := fixture(t)
	var code atomic.Int64
	code.Store(502)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(int(code.Load())) }))
	defer upstream.Close()
	response := decodeValue[map[string]string](t, c.call("POST", "/monitors", monitorPayload(upstream.URL), 201))
	id := response["id"]
	run := func() {
		t.Helper()
		a := decodeValue[map[string]string](t, c.call("POST", "/monitors/"+id+"/checks", map[string]any{}, 202))
		b := decodeValue[map[string]string](t, c.call("POST", "/monitors/"+id+"/checks", map[string]any{}, 202))
		if a["jobId"] != b["jobId"] {
			t.Fatal("duplicate active task")
		}
		var job int64
		s.Store.DB.QueryRow("SELECT id FROM jobs WHERE state='queued'").Scan(&job)
		if e := s.execute(context.Background(), job); e != nil {
			t.Fatal(e)
		}
	}
	run()
	if got := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200)); len(got) != 0 {
		t.Fatal("alert before threshold")
	}
	run()
	alerts := decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	if len(alerts) != 1 || alerts[0].State != "open" {
		t.Fatalf("missing active alert: %+v", alerts)
	}
	aid := idText(alerts[0].ID)
	c.call("POST", "/alerts/"+aid+"/acknowledgements", map[string]int{"version": 1}, 200)
	c.call("POST", "/alerts/"+aid+"/silences", map[string]int{"version": 1, "minutes": 60}, 409)
	c.call("POST", "/alerts/"+aid+"/silences", map[string]int{"version": 2, "minutes": 60}, 200)
	run()
	alerts = decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	if len(alerts) != 1 || alerts[0].State != "acknowledged" || alerts[0].SilenceUntil <= now() {
		t.Fatal("active alert state lost")
	}
	code.Store(200)
	run()
	alerts = decodeValue[[]Alert](t, c.call("GET", "/alerts", nil, 200))
	if alerts[0].State != "resolved" || len(alerts[0].Activity) != 4 {
		t.Fatalf("recovery timeline invalid: %+v", alerts)
	}
	monitors := decodeValue[[]Monitor](t, c.call("GET", "/monitors", nil, 200))
	if len(monitors) != 1 || monitors[0].State != "healthy" || len(monitors[0].History) != 4 || monitors[0].Uptime == nil || *monitors[0].Uptime != 25 {
		t.Fatal("history or uptime invalid")
	}
	c.call("PATCH", "/monitors/"+id, map[string]any{"version": 1, "enabled": false}, 200)
	c.call("POST", "/monitors/"+id+"/checks", map[string]any{}, 422)
	c.call("PATCH", "/monitors/"+id, map[string]any{"version": 1, "enabled": true}, 409)
	dir := s.Store.Dir
	s.Store.Close()
	store, e := OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	s.Store = store
	var count int
	if e = store.DB.QueryRow("SELECT count(*) FROM check_results").Scan(&count); e != nil || count != 4 {
		t.Fatal("restart lost samples")
	}
}
func TestSQLiteLockConstraintsSecretsAndMigration(t *testing.T) {
	dir := t.TempDir()
	s, e := OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if other, e := OpenStore(dir); e == nil {
		other.Close()
		t.Fatal("second instance allowed")
	}
	if _, e = s.DB.Exec("INSERT INTO user_roles VALUES(999,1)"); e == nil {
		t.Fatal("foreign key not enabled")
	}
	enc, e := s.encrypt("sensitive-password")
	if e != nil || strings.Contains(enc, "sensitive-password") {
		t.Fatal("secret not encrypted")
	}
	plain, e := s.decrypt(enc)
	if e != nil || plain != "sensitive-password" {
		t.Fatal("decrypt failed")
	}
	if _, e = s.decrypt(enc + "tampered"); e == nil {
		t.Fatal("tampering accepted")
	}
	var journal string
	s.DB.QueryRow("PRAGMA journal_mode").Scan(&journal)
	if journal != "wal" {
		t.Fatal("WAL not enabled")
	}
	var version int
	s.DB.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version)
	if version != currentSchemaVersion {
		t.Fatal("migration not recorded")
	}
}
func TestPauseDiscardsInflightResult(t *testing.T) {
	s, _, c := fixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.WriteHeader(502) }))
	defer up.Close()
	v := decodeValue[map[string]string](t, c.call("POST", "/monitors", monitorPayload(up.URL), 201))
	c.call("POST", "/monitors/"+v["id"]+"/checks", map[string]any{}, 202)
	var job int64
	s.Store.DB.QueryRow("SELECT id FROM jobs").Scan(&job)
	done := make(chan error, 1)
	go func() { done <- s.execute(context.Background(), job) }()
	<-started
	c.call("PATCH", "/monitors/"+v["id"], map[string]any{"version": 1, "enabled": false}, 200)
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	var count int
	s.Store.DB.QueryRow("SELECT count(*) FROM check_results").Scan(&count)
	if count != 0 {
		t.Fatal("paused target wrote stale result")
	}
}
func TestRealHostSamplingAndScheduler(t *testing.T) {
	s, _, c := fixture(t)
	if e := s.collect(context.Background()); e != nil {
		t.Fatal(e)
	}
	hosts := decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if len(hosts) != 1 || hosts[0].LastSeen == 0 {
		t.Fatal("missing actual host")
	}
	c.call("PATCH", "/hosts/1", map[string]any{"alias": "保存后重读", "version": 1}, 200)
	hosts = decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if hosts[0].Alias != "保存后重读" {
		t.Fatal("alias not persisted")
	}
	c.call("PATCH", "/hosts/1", map[string]any{"alias": "冲突", "version": 1}, 409)
	c.call("GET", "/hosts/1/metrics?hours=1", nil, 200)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer upstream.Close()
	c.call("POST", "/monitors", monitorPayload(upstream.URL), 201)
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	defer func() { cancel(); s.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		s.Store.DB.QueryRow("SELECT count(*) FROM check_results").Scan(&n)
		if n > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scheduler did not execute due monitor")
}

func TestSecretRedactionAndUserManagerScope(t *testing.T) {
	s, ts, c := fixture(t)
	in := monitorPayload("127.0.0.1:3306")
	in.Type = "MySQL"
	in.Username = "monitor"
	in.Password = "database-secret-123"
	c.call("POST", "/monitors", in, 201)
	body := c.call("GET", "/monitors", nil, 200)
	if bytes.Contains(body, []byte(in.Password)) || bytes.Contains(body, []byte(`"secret"`)) {
		t.Fatal("secret exposed to browser")
	}
	var ciphertext string
	s.Store.DB.QueryRow("SELECT secret FROM monitors").Scan(&ciphertext)
	if ciphertext == in.Password || ciphertext == "" {
		t.Fatal("plaintext secret persisted")
	}
	role := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]string{"name": "账号管理员"}, 201))
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"version": 1, "permissions": []string{"user:manage"}}, 200)
	c.call("POST", "/users", userInput{Username: "manager", Name: "成员管理员", Password: "Manager-password-123", Roles: []string{role["id"]}}, 201)
	m := client(t, ts.URL)
	m.login("manager", "Manager-password-123")
	m.call("GET", "/users", nil, 200)
	m.call("GET", "/roles", nil, 200)
	m.call("GET", "/permissions", nil, 200)
	m.call("POST", "/roles", map[string]string{"name": "越权"}, 403)
}
func TestOriginLimitAndExpiredSession(t *testing.T) {
	s, ts, c := fixture(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/roles", strings.NewReader(`{"name":"cross-origin"}`))
	req.Header.Set("Origin", "https://untrusted.invalid")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	res, e := c.c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("cross origin accepted")
	}
	s.Store.DB.Exec("UPDATE sessions SET expires_at=?", now()-1)
	c.call("GET", "/me", nil, 401)
	s.cleanup(context.Background())
	var count int
	s.Store.DB.QueryRow("SELECT count(*) FROM sessions").Scan(&count)
	if count != 0 {
		t.Fatal("expired session not cleaned")
	}
	for i := 0; i < 10; i++ {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = "192.0.2.1:10"
		if !s.allowedLogin(r) {
			t.Fatal("early throttle")
		}
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.1:10"
	if s.allowedLogin(r) {
		t.Fatal("login not limited")
	}
}
