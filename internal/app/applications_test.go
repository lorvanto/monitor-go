package app

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/xml"
	"image"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"
	"github.com/makiuchi-d/gozxing/qrcode"
)

func appForTest(t *testing.T, c *testClient, slug string) string {
	t.Helper()
	return decodeValue[map[string]string](t, c.call("POST", "/applications", map[string]any{"slug": slug, "name": slug, "enabled": true}, 200))["id"]
}
func keyForTest(t *testing.T, c *testClient, id string, scopes ...string) (string, string) {
	t.Helper()
	v := decodeValue[map[string]any](t, c.call("POST", "/applications/"+id+"/keys", map[string]any{"name": "test-key", "days": 90, "scopes": scopes}, 200))
	return v["id"].(string), v["token"].(string)
}
func machineForTest(t *testing.T, base, token, method, path string, want int, headers ...string) ([]byte, http.Header) {
	t.Helper()
	req, e := http.NewRequest(method, base+"/api/v1"+path, nil)
	if e != nil {
		t.Fatal(e)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	body, e := io.ReadAll(res.Body)
	if e != nil {
		t.Fatal(e)
	}
	if res.StatusCode != want {
		t.Fatalf("%s %s returned %d, expected %d: %s", method, path, res.StatusCode, want, body)
	}
	return body, res.Header
}
func TestApplicationKeyLifecycleAndBoundary(t *testing.T) {
	s, ts, c := fixture(t)
	id := appForTest(t, c, "ops-app")
	keyID, token := keyForTest(t, c, id, "host:read")
	var hash, prefix string
	s.Store.DB.QueryRow("SELECT secret_hash,key_prefix FROM api_keys WHERE id=?", keyID).Scan(&hash, &prefix)
	if hash != digest(token) || strings.Contains(hash, token) || len(prefix) != 12 {
		t.Fatal("key is not hash-only")
	}
	machineForTest(t, ts.URL, token, "GET", "/integrations/me", 200)
	machineForTest(t, ts.URL, token, "GET", "/integrations/hosts", 200)
	machineForTest(t, ts.URL, token, "GET", "/integrations/monitors", 403)
	machineForTest(t, ts.URL, token, "GET", "/hosts", 401)
	c.call("GET", "/integrations/me", nil, 401)
	machineForTest(t, ts.URL, token, "GET", "/integrations/me?token=unused", 401)
	machineForTest(t, ts.URL, "", "GET", "/integrations/me", 401, "Authorization", token)
	raw := c.call("GET", "/applications/"+id+"/keys", nil, 200)
	if bytes.Contains(raw, []byte(token)) || bytes.Contains(raw, []byte(hash)) {
		t.Fatal("key secret leaked")
	}
	var last int64
	s.Store.DB.QueryRow("SELECT last_used_at FROM api_keys WHERE id=?", keyID).Scan(&last)
	if last == 0 {
		t.Fatal("last use not recorded")
	}
	rotated := decodeValue[map[string]any](t, c.call("POST", "/applications/"+id+"/keys", map[string]any{"name": "rotated", "days": 1, "scopes": []string{"host:read"}, "rotateId": keyID, "version": 1}, 200))
	newToken := rotated["token"].(string)
	machineForTest(t, ts.URL, token, "GET", "/integrations/me", 401)
	machineForTest(t, ts.URL, newToken, "GET", "/integrations/me", 200)
	c.call("PATCH", "/applications/"+id, map[string]any{"slug": "ops-app", "name": "ops-app", "enabled": false, "version": 1}, 200)
	machineForTest(t, ts.URL, newToken, "GET", "/integrations/me", 401)
	c.call("POST", "/applications/"+id+"/keys", map[string]any{"name": "denied", "days": 1, "scopes": []string{"host:read"}}, 422)
	c.call("PATCH", "/applications/"+id, map[string]any{"slug": "ops-app", "name": "ops-app", "enabled": true, "version": 2}, 200)
	machineForTest(t, ts.URL, newToken, "GET", "/integrations/me", 200)
	c.call("DELETE", "/application-keys/"+rotated["id"].(string), map[string]int{"version": 1}, 200)
	machineForTest(t, ts.URL, newToken, "GET", "/integrations/me", 401)
	expiredID, expired := keyForTest(t, c, id, "host:read")
	s.Store.DB.Exec("UPDATE api_keys SET expires_at=? WHERE id=?", now()-1, expiredID)
	machineForTest(t, ts.URL, expired, "GET", "/integrations/me", 401)
	c.call("POST", "/applications/"+id+"/keys", map[string]any{"name": "invalid", "days": 90, "scopes": []string{"role:manage"}}, 422)
	c.call("DELETE", "/applications/"+id, map[string]int{"version": 3}, 200)
	c.call("POST", "/applications", map[string]any{"slug": "ops-app", "name": "duplicate", "enabled": true}, 422)
	var leaked int
	s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE instr(resource,?)>0 OR instr(resource,?)>0", token, newToken).Scan(&leaked)
	if leaked != 0 {
		t.Fatal("secret in audit")
	}
}
func TestReleaseLifecycleDownloadAndFileReferences(t *testing.T) {
	s, ts, c := fixture(t)
	a := appForTest(t, c, "first-app")
	b := appForTest(t, c, "other-app")
	_, token := keyForTest(t, c, a, "release:read", "release:download")
	_, other := keyForTest(t, c, b, "release:read", "release:download")
	folder := folderForTest(t, c, "1", "构建产物")
	f := uploadForTest(t, c, folder, "app.zip", []byte("verified-package-content"), 200)
	in := releaseInput{ApplicationID: a, Name: "运维端", VersionName: "1.0.0", Platform: "linux", Arch: "amd64", Source: "file", FileID: f.ID, State: "draft"}
	id := decodeValue[map[string]string](t, c.call("POST", "/releases", in, 200))["id"]
	c.call("GET", "/releases/"+id+"/download", nil, 404)
	c.call("DELETE", "/files/"+f.ID, map[string]int{"version": 1}, 422)
	c.call("DELETE", "/files/"+folder, map[string]int{"version": 1}, 422)
	c.call("DELETE", "/applications/"+a, map[string]int{"version": 1}, 422)
	in.Version = 1
	in.State = "published"
	c.call("PATCH", "/releases/"+id, in, 200)
	data, h := machineForTest(t, ts.URL, token, "GET", "/integrations/releases/"+id+"/download", 200)
	if string(data) != "verified-package-content" || h.Get("X-Checksum-SHA256") != digest(string(data)) {
		t.Fatal("artifact checksum mismatch")
	}
	data, _ = machineForTest(t, ts.URL, token, "GET", "/integrations/releases/"+id+"/download", 206, "Range", "bytes=0-7")
	if string(data) != "verified" {
		t.Fatal("range mismatch")
	}
	machineForTest(t, ts.URL, token, "HEAD", "/integrations/releases/"+id+"/download", 200)
	machineForTest(t, ts.URL, token, "GET", "/integrations/releases/"+id+"/download", 416, "Range", "bytes=1000-2000")
	machineForTest(t, ts.URL, other, "GET", "/integrations/releases/"+id+"/download", 404)
	v, e := readRelease(s.Store.DB, id)
	if e != nil || v.DownloadCount != 2 {
		t.Fatal("invalid download count", v.DownloadCount, e)
	}
	raw, _ := machineForTest(t, ts.URL, other, "GET", "/integrations/releases?applicationId="+a, 200)
	if bytes.Contains(raw, []byte("运维端")) {
		t.Fatal("cross-application release visible")
	}
	in.Version = 2
	in.VersionName = "2.0.0"
	c.call("PATCH", "/releases/"+id, in, 422)
	in.VersionName = "1.0.0"
	in.State = "draft"
	c.call("PATCH", "/releases/"+id, in, 422)
	in.State = "withdrawn"
	c.call("PATCH", "/releases/"+id, in, 200)
	machineForTest(t, ts.URL, token, "GET", "/integrations/releases/"+id+"/download", 404)
	c.call("DELETE", "/releases/"+id, map[string]int{"version": 3}, 200)
	c.call("DELETE", "/files/"+folder, map[string]int{"version": 1}, 200)
	in.Source = "url"
	in.FileID = ""
	in.DownloadURL = "https://example.com/package.zip"
	in.VersionName = "2.0.0"
	in.State = "published"
	in.Version = 0
	id = decodeValue[map[string]string](t, c.call("POST", "/releases", in, 200))["id"]
	raw, h = machineForTest(t, ts.URL, token, "GET", "/integrations/releases/"+id+"/download", 200)
	if h.Get("Location") != "" || !bytes.Contains(raw, []byte("https://example.com/package.zip")) {
		t.Fatal("machine external download must not redirect bearer")
	}
	v, _ = readRelease(s.Store.DB, id)
	if v.DownloadCount != 0 {
		t.Fatal("URL lookup is not a download")
	}
	in.VersionName = "3"
	in.DownloadURL = "http://example.com/file"
	c.call("POST", "/releases", in, 422)
	in.DownloadURL = "https://user:pass@example.com/file"
	c.call("POST", "/releases", in, 422)
}
func TestKeyLimitRotationAndReadOnlyExternalURLs(t *testing.T) {
	s, ts, c := fixture(t)
	a := appForTest(t, c, "key-limit")
	var oldID, oldToken string
	for i := 0; i < 10; i++ {
		id, token := keyForTest(t, c, a, "release:read")
		if i == 0 {
			oldID, oldToken = id, token
		}
	}
	c.call("POST", "/applications/"+a+"/keys", map[string]any{"name": "over-limit", "days": 10, "scopes": []string{"release:read"}}, 422)
	c.call("POST", "/applications/"+a+"/keys", map[string]any{"name": "rotate-at-limit", "days": 10, "scopes": []string{"release:read"}, "rotateId": oldID, "version": 99}, 409)
	machineForTest(t, ts.URL, oldToken, "GET", "/integrations/me", 200)
	c.call("POST", "/applications/"+a+"/keys", map[string]any{"name": "rotate-at-limit", "days": 10, "scopes": []string{"release:read"}, "rotateId": oldID, "version": 1}, 200)
	machineForTest(t, ts.URL, oldToken, "GET", "/integrations/me", 401)
	in := releaseInput{ApplicationID: a, Name: "External", VersionName: "1.0", Platform: "other", Arch: "other", Source: "url", DownloadURL: "https://example.com/private.zip", State: "published"}
	id := decodeValue[map[string]string](t, c.call("POST", "/releases", in, 200))["id"]
	c.c.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	res, e := c.c.Get(c.base + "/api/v1/releases/" + id + "/download")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 302 || res.Header.Get("Location") != in.DownloadURL {
		t.Fatal("browser external link did not redirect")
	}
	c.c.CheckRedirect = nil
	v, e := readRelease(s.Store.DB, id)
	if e != nil || v.DownloadCount != 1 {
		t.Fatal("redirect count mismatch", e)
	}
	role := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]string{"name": "版本只读"}, 201))
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"permissions": []string{"release:read"}, "version": 1}, 200)
	c.call("POST", "/users", userInput{Username: "release-read", Name: "版本只读", Password: "Read-password-123!", Roles: []string{role["id"]}}, 201)
	reader := client(t, ts.URL)
	reader.login("release-read", "Read-password-123!")
	raw := reader.call("GET", "/releases", nil, 200)
	if bytes.Contains(raw, []byte(in.DownloadURL)) {
		t.Fatal("download URL leaked to read-only metadata permission")
	}
	reader.call("GET", "/releases/"+id+"/download", nil, 403)
	reader.call("POST", "/tools/codes", codeInput{}, 403)
}

func TestApplicationPermissionsAndCodeEndpoints(t *testing.T) {
	_, ts, c := fixture(t)
	a := appForTest(t, c, "admin-app")
	r := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]any{"name": "版本编辑", "description": ""}, 201))
	c.call("PATCH", "/roles/"+r["id"], map[string]any{"permissions": []string{"release:manage", "tool:generate"}, "version": 1}, 200)
	c.call("POST", "/users", userInput{Username: "release-user", Name: "版本编辑", Password: "Read-password-123!", Roles: []string{r["id"]}}, 201)
	limited := client(t, ts.URL)
	limited.login("release-user", "Read-password-123!")
	limited.call("GET", "/applications", nil, 200)
	limited.call("GET", "/applications/"+a+"/keys", nil, 403)
	limited.call("POST", "/applications/"+a+"/keys", map[string]any{}, 403)
	limited.call("GET", "/releases", nil, 200)
	f := uploadForTest(t, c, "2", "private.zip", []byte("private"), 200)
	in := releaseInput{ApplicationID: a, Name: "test", VersionName: "1", Platform: "linux", Arch: "amd64", Source: "file", FileID: f.ID, State: "draft"}
	limited.call("POST", "/releases", in, 403)
	out := decodeValue[codeResult](t, limited.call("POST", "/tools/codes", codeInput{Kind: "qr", Content: "内部协作", Width: 256, Level: "M"}, 200))
	if out.PNG == "" || out.SVG == "" {
		t.Fatal("code endpoint returned empty images")
	}
	guest := client(t, ts.URL)
	guest.call("POST", "/tools/codes", codeInput{}, 401)
}
func TestCodeImagesDecodeAndInputSafety(t *testing.T) {
	cases := []struct {
		kind, content, expected string
		width                   int
		reader                  gozxing.Reader
	}{{"qr", "中文测试 https://example.com/路径?a=1&b=2", "", 400, qrcode.NewQRCodeReader()}, {"qr", "<script>alert('x')</script>", "", 400, qrcode.NewQRCodeReader()}, {"code128", "MonitorGo-2026 42", "", 800, oned.NewCode128Reader()}, {"code39", "MONITOR-GO-2026", "", 800, oned.NewCode39Reader()}, {"ean13", "690123456789", "6901234567892", 400, oned.NewEAN13Reader()}, {"ean8", "9638507", "96385074", 256, oned.NewEAN8Reader()}}
	for _, tc := range cases {
		t.Run(tc.kind+tc.content, func(t *testing.T) {
			out, e := renderCode(codeInput{Kind: tc.kind, Content: tc.content, Width: tc.width, Height: 160, Level: "M", Label: true})
			if e != nil {
				t.Fatal(e)
			}
			raw, e := base64.StdEncoding.DecodeString(out.PNG)
			if e != nil {
				t.Fatal(e)
			}
			img, e := png.Decode(bytes.NewReader(raw))
			if e != nil {
				t.Fatal(e)
			}
			bitmap, e := gozxing.NewBinaryBitmapFromImage(img)
			if e != nil {
				t.Fatal(e)
			}
			got, e := tc.reader.Decode(bitmap, nil)
			if e != nil {
				t.Fatal("image does not decode", e)
			}
			want := tc.expected
			if want == "" {
				want = tc.content
			}
			if got.GetText() != want || out.Content != want {
				t.Fatalf("decoded %q expected %q", got.GetText(), want)
			}
			svg, e := base64.StdEncoding.DecodeString(out.SVG)
			if e != nil {
				t.Fatal(e)
			}
			d := xml.NewDecoder(bytes.NewReader(svg))
			for {
				token, e := d.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					t.Fatal("invalid SVG", e)
				}
				if v, ok := token.(xml.StartElement); ok && v.Name.Local != "svg" && v.Name.Local != "rect" && v.Name.Local != "path" {
					t.Fatal("unsafe SVG element")
				}
			}
			if bytes.Contains(svg, []byte("<script")) {
				t.Fatal("unescaped content in SVG")
			}
			if img.Bounds() != image.Rect(0, 0, out.Width, out.Height) {
				t.Fatal("dimensions mismatch")
			}
		})
	}
	invalid := []codeInput{{Kind: "qr", Content: strings.Repeat("界", 401), Width: 2048, Level: "M"}, {Kind: "qr", Content: strings.Repeat("abcdef", 100), Width: 128, Level: "H"}, {Kind: "ean13", Content: "6901234567893", Width: 400, Height: 160}, {Kind: "code128", Content: "汉字", Width: 800, Height: 160}, {Kind: "code39", Content: "lowercase", Width: 800, Height: 160}, {Kind: "qr", Content: "valid", Width: 3000, Level: "M"}, {Kind: "qr", Content: "valid", Width: 256, Level: "unknown"}, {Kind: "code128", Content: "control\n", Width: 800, Height: 160}}
	for _, in := range invalid {
		if _, e := renderCode(in); e == nil {
			t.Fatal("invalid code input accepted", in.Kind)
		}
	}
}
func TestV3UpgradeAddsApplicationsWithoutMenuCollision(t *testing.T) {
	dir := t.TempDir()
	db, e := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schema + "\n" + migration2 + "\n" + migration3); e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO roles(id,name,builtin) VALUES(1,'管理员',1),(2,'成员',1); INSERT INTO role_permissions VALUES(1,'role:manage'),(2,'host:read'); INSERT INTO users(id,username,display_name,password_hash) VALUES(1,'legacy','旧用户','hash'); INSERT INTO user_roles VALUES(1,1); INSERT INTO sessions VALUES('old',1,'csrf',1,9999999999999); INSERT INTO menus(id,name,kind) VALUES(19,'应用与工具','group');`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	st, e := OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	var ver, count, keys int
	st.DB.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&ver)
	st.DB.QueryRow("SELECT count(*) FROM menus WHERE name='应用与工具'").Scan(&count)
	st.DB.QueryRow("SELECT count(*) FROM role_permissions WHERE role_id=1 AND permission IN ('application:read','application:manage','application:keys','release:read','release:manage','release:download','tool:generate')").Scan(&keys)
	if ver != 4 || count != 2 || keys != 7 {
		t.Fatal("upgrade mismatch", ver, count, keys)
	}
	st.DB.QueryRow("SELECT count(*) FROM role_permissions WHERE role_id=2").Scan(&count)
	if count != 1 {
		t.Fatal("member permissions changed")
	}
	st.DB.QueryRow("SELECT count(*) FROM sessions").Scan(&count)
	if count != 0 {
		t.Fatal("admin session not rotated")
	}
	st.Close()
	st, e = OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	st.DB.QueryRow("SELECT count(*) FROM menus WHERE builtin_key='application-tools'").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate upgrade menus")
	}
}
