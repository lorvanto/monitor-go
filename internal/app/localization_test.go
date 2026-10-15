package app

import (
	"bytes"
	"encoding/json"
	"github.com/lorvanto/monitor-go/locales"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLocalizedHTTPAndPersistedNames(t *testing.T) {
	s, ts, c := fixture(t) // Real SQLite file, authenticated HTTP, schema migrations.
	for _, code := range locales.Supported {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/me", nil)
		req.Header.Set("Accept-Language", code)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 401 || res.Header.Get("Content-Language") != code || !strings.Contains(res.Header.Get("Vary"), "Accept-Language") || !bytes.Contains(raw, []byte(locales.Text(code, "请先登录"))) {
			t.Fatalf("%s: %d %s", code, res.StatusCode, raw)
		}
	}
	// Static UI translation must never rename an administrator's navigation data.
	if _, err := s.Store.DB.Exec("UPDATE menus SET name='Custom 日本語 한국어 繁體',version=version+1 WHERE page_key='overview'"); err != nil {
		t.Fatal(err)
	}
	body := c.call("GET", "/menus", nil, 200)
	var result struct {
		Items []Menu `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Items {
		if item.PageKey == "overview" && (item.Name != "Custom 日本語 한국어 繁體" || item.NameKey != "") {
			t.Fatalf("custom name changed: %#v", item)
		}
		if item.PageKey == "host" && item.NameKey != "主机详情" {
			t.Fatalf("default menu lacks translation key: %#v", item)
		}
	}
	// Parameterized validation retains its values and status in the selected language.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/tools/codes", strings.NewReader(`{"kind":"ean13","content":"1","width":512,"height":160,"level":"M"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "ja-JP")
	req.Header.Set("X-CSRF-Token", c.csrf)
	res, err := c.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 422 || !bytes.Contains(raw, []byte("12")) || !bytes.Contains(raw, []byte("13")) || bytes.Contains(raw, []byte("请输入")) {
		t.Fatalf("parameterized error: %d %s", res.StatusCode, raw)
	}
}

func TestPurgeConfirmationIsIndependentOfLocale(t *testing.T) {
	s, _, c := fixture(t)
	folder := folderForTest(t, c, "1", "QA 日本語 한국어 繁體")
	c.call("DELETE", "/files/"+folder, map[string]int{"version": 1}, 200)
	for _, code := range []string{"en-US", "ja-JP", "ko-KR", "zh-TW"} {
		body, _ := json.Marshal(map[string]any{"version": 2, "confirm": locales.Text(code, "永久删除")})
		req, _ := http.NewRequest("POST", c.base+"/api/v1/files/"+folder+"/purge", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", code)
		req.Header.Set("X-CSRF-Token", c.csrf)
		res, err := c.c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 422 || res.Header.Get("Content-Language") != code {
			t.Fatalf("localized UI phrase is not a wire token: %s: %d", code, res.StatusCode)
		}
	}
	c.call("POST", "/files/"+folder+"/purge", map[string]any{"version": 2, "confirm": "DELETE"}, 200)
	var purgedAt int64
	if err := s.Store.DB.QueryRow("SELECT purged_at FROM files WHERE id=?", folder).Scan(&purgedAt); err != nil || purgedAt == 0 {
		t.Fatalf("fixed confirmation did not purge: %d, %v", purgedAt, err)
	}
}
