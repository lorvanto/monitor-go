package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPKeywordAndLatencyRules(t *testing.T) {
	for _, tc := range []struct {
		name, body, keyword string
		invert              bool
		status, threshold   int
		delay               time.Duration
		state, detail       string
	}{
		{"unicode hit", "服务正常 secret-body", "服务正常", false, 200, 0, 0, "healthy", "matched"},
		{"miss", "sensitive-body", "missing", false, 200, 0, 0, "failed", "required"},
		{"invert hit", "sensitive-body", "sensitive", true, 200, 0, 0, "failed", "forbidden"},
		{"invert miss", "sensitive-body", "missing", true, 200, 0, 0, "healthy", "matched"},
		{"case sensitive", "Healthy", "healthy", false, 200, 0, 0, "failed", "required"},
		{"truncated", strings.Repeat("x", 1<<20) + "beyond-limit", "beyond-limit", false, 200, 0, 0, "failed", "truncated"},
		{"bounded inverse", strings.Repeat("x", 1<<20) + "beyond-limit", "beyond-limit", true, 200, 0, 0, "healthy", "truncated"},
		{"slow success", "ok", "ok", false, 200, 1, 20 * time.Millisecond, "warning", "exceeds"},
		{"keyword before latency", "no", "ok", false, 200, 1, 20 * time.Millisecond, "failed", "required"},
		{"status before keyword", "no", "ok", false, 503, 1, 20 * time.Millisecond, "failed", "HTTP 503"},
		{"old status only", "no", "", false, 200, 0, 0, "healthy", "HTTP 200"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tc.delay)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			got := probe(ctx, Monitor{Type: "HTTP", Target: server.URL, Timeout: 2, Expected: 200, Keyword: tc.keyword, KeywordInvert: tc.invert, LatencyWarnMS: tc.threshold}, "")
			if got.State != tc.state || !strings.Contains(got.Detail, tc.detail) {
				t.Fatalf("%+v", got)
			}
			if strings.Contains(got.Detail, "sensitive-body") || strings.Contains(got.Detail, "secret-body") || strings.Contains(got.Detail, "beyond-limit") {
				t.Fatal("body leaked")
			}
			if tc.status != 200 && strings.Contains(got.Detail, "required") {
				t.Fatal("keyword failure overrode HTTP failure")
			}
		})
	}
}
func TestHTTPKeywordBodyTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.(http.Flusher).Flush(); <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got := probe(ctx, Monitor{Type: "HTTP", Target: server.URL, Expected: 200, Timeout: 1, Keyword: "ok", LatencyWarnMS: 1}, "")
	if got.State != "failed" || got.Detail != "检测超时" {
		t.Fatalf("%+v", got)
	}
}
func TestMigrationSevenHTTPDefaultsAndConstraints(t *testing.T) {
	dir, db := versionFourDatabase(t)
	if _, err := db.Exec(migration5 + migration6 + `INSERT INTO monitors(name,type,target,group_name,interval_seconds,timeout_seconds) VALUES('legacy','HTTP','http://example.com','test',60,5)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m, err := scanMonitor(store.DB.QueryRow("SELECT " + monitorColumns + " FROM monitors WHERE name='legacy'"))
	if err != nil || m.Keyword != "" || m.KeywordInvert || m.LatencyWarnMS != 0 {
		t.Fatalf("defaults: %+v %v", m, err)
	}
	for _, q := range []string{"UPDATE monitors SET keyword_invert=2", "UPDATE monitors SET latency_warn_ms=-1", "UPDATE monitors SET latency_warn_ms=30001", "UPDATE monitors SET keyword=zeroblob(1025)"} {
		if _, err = store.DB.Exec(q); err == nil {
			t.Fatalf("constraint missing: %s", q)
		}
	}
}
func TestHTTPRulesCreateEditAndTypeChange(t *testing.T) {
	_, _, c := fixture(t)
	in := monitorPayload("http://127.0.0.1:1")
	in.Keyword = "healthy 世界"
	in.KeywordInvert = true
	in.LatencyWarnMS = 400
	id := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))["id"]
	monitors := decodeValue[[]Monitor](t, c.call("GET", "/monitors", nil, 200))
	m := monitors[0]
	if m.Keyword != in.Keyword || !m.KeywordInvert || m.LatencyWarnMS != 400 {
		t.Fatalf("create: %+v", m)
	}
	in.Keyword = "changed"
	in.KeywordInvert = false
	in.LatencyWarnMS = 250
	c.call("PUT", "/monitors/"+id, monitorEditInput{monitorInput: in, Version: m.Version}, 200)
	monitors = decodeValue[[]Monitor](t, c.call("GET", "/monitors", nil, 200))
	m = monitors[0]
	if m.Keyword != "changed" || m.KeywordInvert || m.LatencyWarnMS != 250 {
		t.Fatalf("edit: %+v", m)
	}
	in.Keyword = strings.Repeat("界", 342)
	c.call("PUT", "/monitors/"+id, monitorEditInput{monitorInput: in, Version: m.Version}, 422)
	in.Keyword = "changed"
	in.Type = "TCP"
	in.Target = "127.0.0.1:1"
	c.call("PUT", "/monitors/"+id, monitorEditInput{monitorInput: in, Version: m.Version}, 200)
	monitors = decodeValue[[]Monitor](t, c.call("GET", "/monitors", nil, 200))
	m = monitors[0]
	if m.Keyword != "" || m.KeywordInvert || m.LatencyWarnMS != 0 {
		t.Fatalf("non-HTTP retained rules: %+v", m)
	}
}
