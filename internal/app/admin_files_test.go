package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func uploadForTest(t *testing.T, c *testClient, parent, name string, data []byte, want int) FileEntry {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, e := writer.CreateFormFile("file", name)
	if e != nil {
		t.Fatal(e)
	}
	part.Write(data)
	writer.Close()
	r, _ := http.NewRequest("POST", c.base+"/api/v1/files/uploads?parent="+parent, &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("X-CSRF-Token", c.csrf)
	res, e := c.c.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("upload %s status %d want %d: %s", name, res.StatusCode, want, raw)
	}
	var v struct{ Data FileEntry }
	json.Unmarshal(raw, &v)
	return v.Data
}
func binaryForTest(t *testing.T, c *testClient, path string, want int) ([]byte, http.Header) {
	t.Helper()
	res, e := c.c.Get(c.base + "/api/v1" + path)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("GET %s status %d want %d: %s", path, res.StatusCode, want, raw)
	}
	return raw, res.Header
}
func folderForTest(t *testing.T, c *testClient, parent, name string) string {
	t.Helper()
	v := decodeValue[map[string]string](t, c.call("POST", "/files/folders", map[string]string{"name": name, "parentId": parent}, 200))
	return v["id"]
}
func TestFilesUploadDownloadTrashRestoreAndPurge(t *testing.T) {
	s, _, c := fixture(t)
	folder := folderForTest(t, c, "1", "项目文档")
	nested := folderForTest(t, c, folder, "图片")
	f := uploadForTest(t, c, nested, "说明.txt", []byte("Hello，团队\n<script>alert(1)</script>"), 200)
	raw, headers := binaryForTest(t, c, "/files/"+f.ID+"/download", 200)
	if string(raw) != "Hello，团队\n<script>alert(1)</script>" || !strings.HasPrefix(headers.Get("Content-Disposition"), "attachment") || headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("download changed bytes or unsafe headers")
	}
	if len(f.SHA256) != 64 || f.Size != int64(len(raw)) {
		t.Fatal("bad metadata")
	}
	info, e := os.Stat(filepath.Join(s.Store.Dir, "uploads", "objects"))
	if e != nil || info.Mode().Perm() != 0700 {
		t.Fatal("storage permissions")
	}
	preview := decodeValue[map[string]any](t, c.call("GET", "/files/"+f.ID+"/preview", nil, 200))
	if preview["kind"] != "text" || preview["text"] != string(raw) {
		t.Fatal("text preview")
	}
	uploadForTest(t, c, nested, "说明.txt", []byte("duplicate"), 422)
	c.call("PATCH", "/files/"+folder, map[string]any{"name": "不可循环", "parentId": nested, "version": 1}, 422)
	c.call("PATCH", "/files/"+f.ID, map[string]any{"name": "../escape", "parentId": nested, "version": 1}, 422)
	c.call("PATCH", "/files/"+f.ID, map[string]any{"name": "更新.txt", "parentId": nested, "version": 9}, 409)
	c.call("DELETE", "/files/2", map[string]int{"version": 1}, 422)
	c.call("DELETE", "/files/"+folder, map[string]int{"version": 1}, 200)
	binaryForTest(t, c, "/files/"+f.ID+"/download", 404)
	trash := decodeValue[struct{ Items []FileEntry }](t, c.call("GET", "/files?trash=1", nil, 200))
	if len(trash.Items) != 1 || trash.Items[0].ID != folder {
		t.Fatal("trash batch root missing")
	}
	c.call("POST", "/files/"+folder+"/restore", map[string]any{"version": 2, "parentId": nested}, 422)
	folderForTest(t, c, "1", "项目文档")
	c.call("POST", "/files/"+folder+"/restore", map[string]any{"version": 2, "parentId": "1"}, 422)
	c.call("POST", "/files/"+folder+"/restore", map[string]any{"version": 2, "parentId": "1", "name": "恢复文档"}, 200)
	binaryForTest(t, c, "/files/"+f.ID+"/download", 200)
	c.call("DELETE", "/files/"+folder, map[string]int{"version": 3}, 200)
	c.call("POST", "/files/"+folder+"/purge", map[string]any{"version": 4, "confirm": "yes"}, 422)
	c.call("POST", "/files/"+folder+"/purge", map[string]any{"version": 4, "confirm": "永久删除"}, 200)
	var key string
	var purge int64
	if e = s.Store.DB.QueryRow("SELECT object_key,purged_at FROM files WHERE id=?", f.ID).Scan(&key, &purge); e != nil || key != "" || purge == 0 {
		t.Fatal("purge tombstone or GC failed", e)
	}
	count, e := s.Store.Files.Open("objects")
	if e != nil {
		t.Fatal(e)
	}
	entries, _ := count.ReadDir(-1)
	count.Close()
	if len(entries) != 0 {
		t.Fatal("purged/duplicate objects leaked")
	}
}
func TestFileImageStylesCacheAndContentSafety(t *testing.T) {
	s, _, c := fixture(t)
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for x := 0; x < 80; x++ {
		for y := 0; y < 40; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 3), 80, 180, 255})
		}
	}
	var encoded bytes.Buffer
	png.Encode(&encoded, img)
	f := uploadForTest(t, c, "2", "sample.png", encoded.Bytes(), 200)
	if f.Width != 80 || f.Height != 40 {
		t.Fatal("image metadata missing")
	}
	style := decodeValue[map[string]string](t, c.call("POST", "/image-styles", ImageStyle{Name: "正方形", Width: 32, Height: 32, Fit: "cover"}, 200))
	raw, h := binaryForTest(t, c, "/files/"+f.ID+"/image?style="+style["id"], 200)
	config, _, e := image.DecodeConfig(bytes.NewReader(raw))
	if e != nil || config.Width != 32 || config.Height != 32 || h.Get("Content-Type") != "image/png" {
		t.Fatal("crop failed", e)
	}
	second, _ := binaryForTest(t, c, "/files/"+f.ID+"/image?style="+style["id"], 200)
	if !bytes.Equal(second, raw) {
		t.Fatal("cache changed bytes")
	}
	c.call("PATCH", "/image-styles/"+style["id"], ImageStyle{Name: "等比缩略图", Width: 32, Height: 32, Fit: "contain", Version: 1}, 200)
	raw, _ = binaryForTest(t, c, "/files/"+f.ID+"/image?style="+style["id"], 200)
	config, _, e = image.DecodeConfig(bytes.NewReader(raw))
	if e != nil || config.Width != 32 || config.Height != 16 {
		t.Fatal("contain or cache version failed", config, e)
	}
	cleared := decodeValue[map[string]int](t, c.call("POST", "/files/cache/clear", map[string]any{}, 200))
	if cleared["removed"] != 2 {
		t.Fatalf("wrong cache clear count %v", cleared)
	}
	c.call("DELETE", "/image-styles/"+style["id"], map[string]int{"version": 2}, 200)
	binaryForTest(t, c, "/files/"+f.ID+"/image?style="+style["id"], 404)
	html := uploadForTest(t, c, "2", "unsafe.html", []byte("<!doctype html><script>alert(1)</script>"), 200)
	preview := decodeValue[map[string]any](t, c.call("GET", "/files/"+html.ID+"/preview", nil, 200))
	if preview["kind"] != "text" {
		t.Fatal("HTML must be escaped text preview")
	}
	binaryForTest(t, c, "/files/"+html.ID+"/image", 422)
	// A locally replaced object cannot escape the storage root through a symlink.
	stored, e := readFile(s.Store.DB, f.ID)
	if e != nil {
		t.Fatal(e)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("not allowed"), 0600)
	s.Store.Files.Remove("objects/" + stored.ObjectKey)
	if e = os.Symlink(outside, filepath.Join(s.Store.Dir, "uploads", "objects", stored.ObjectKey)); e != nil {
		t.Fatal(e)
	}
	binaryForTest(t, c, "/files/"+f.ID+"/download", 500)
}
func TestFileLimitsSettingsAndAuditRetention(t *testing.T) {
	s, _, c := fixture(t)
	settings := decodeValue[Settings](t, c.call("GET", "/settings", nil, 200))
	settings.InstanceName = "研发工作台"
	settings.UploadLimitMB = 1
	settings.StorageLimitMB = 10
	settings.AuditDays = 7
	c.call("PATCH", "/settings", settings, 200)
	c.call("PATCH", "/settings", settings, 409)
	nav := decodeValue[map[string]any](t, c.call("GET", "/me/navigation", nil, 200))
	if nav["instanceName"] != "研发工作台" {
		t.Fatal("instance name not updated")
	}
	uploadForTest(t, c, "2", "empty.txt", nil, 422)
	uploadForTest(t, c, "2", "too-big.txt", bytes.Repeat([]byte("x"), 1048577), 422)
	csrf := c.csrf
	c.csrf = ""
	uploadForTest(t, c, "2", "forbidden.txt", []byte("csrf"), 403)
	c.csrf = csrf
	f := uploadForTest(t, c, "2", "quota.txt", []byte("quota"), 200)
	s.Store.DB.Exec("UPDATE files SET size=? WHERE id=?", 10*1048576, f.ID)
	uploadForTest(t, c, "2", "beyond.txt", []byte("full"), 422)
	records := decodeValue[struct {
		Items []struct {
			RequestID string `json:"requestId"`
			Path      string `json:"path"`
			Action    string `json:"action"`
		}
	}](t, c.call("GET", "/audit?q=上传文件", nil, 200))
	if len(records.Items) != 1 || records.Items[0].RequestID == "" || records.Items[0].Path != "/api/v1/files/uploads" {
		t.Fatal("audit request context missing")
	}
	s.Store.DB.Exec("INSERT INTO audit_logs(at,actor,action,resource) VALUES(?,?,?,?)", now()-8*86400000, "old", "旧操作", "")
	s.cleanup(context.Background())
	var count int
	s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE actor='old'").Scan(&count)
	if count != 0 {
		t.Fatal("audit retention ignored")
	}
}
func TestMenuTreeConflictsResetAndPermissionFiltering(t *testing.T) {
	_, ts, c := fixture(t)
	input := menuInput{Name: "文档", Kind: "group", Icon: "BookOpen", Visible: true, Sort: 1}
	group := decodeValue[map[string]string](t, c.call("POST", "/menus", input, 200))
	input.ParentID = group["id"]
	input.Kind = "link"
	input.URL = "javascript:alert(1)"
	c.call("POST", "/menus", input, 422)
	input.URL = "https://example.com/docs"
	link := decodeValue[map[string]string](t, c.call("POST", "/menus", input, 200))
	input.Kind = "group"
	input.ParentID = group["id"]
	input.Name = "子目录"
	child := decodeValue[map[string]string](t, c.call("POST", "/menus", input, 200))
	input.Version = 1
	input.ParentID = child["id"]
	c.call("PATCH", "/menus/"+group["id"], input, 422)
	c.call("DELETE", "/menus/"+group["id"], map[string]int{"version": 1}, 422)
	c.call("DELETE", "/menus/10", map[string]int{"version": 1}, 422)
	input = menuInput{Name: "文档链接", Kind: "link", Icon: "Link", URL: "https://example.com/guide", Visible: false, Sort: 5, Version: 1, Permission: "menu:manage"}
	c.call("PATCH", "/menus/"+link["id"], input, 200)
	c.call("PATCH", "/menus/"+link["id"], input, 409)
	before := decodeValue[struct {
		Items    []Menu
		Revision string
	}](t, c.call("GET", "/menus", nil, 200))
	c.call("POST", "/menus/reset", map[string]string{"revision": "old"}, 409)
	c.call("POST", "/menus/reset", map[string]string{"revision": before.Revision}, 200)
	after := decodeValue[struct {
		Items    []Menu
		Revision string
	}](t, c.call("GET", "/menus", nil, 200))
	if len(after.Items) != 19 || after.Revision == before.Revision {
		t.Fatal("reset lost custom menus")
	}
	c.call("POST", "/users", userInput{Username: "viewer", Name: "观察者", Password: "Read-password-123!", Roles: []string{"3"}}, 201)
	reader := client(t, ts.URL)
	reader.login("viewer", "Read-password-123!")
	reader.call("GET", "/menus", nil, 403)
	reader.call("GET", "/audit", nil, 403)
	reader.call("GET", "/settings", nil, 403)
	reader.call("GET", "/files", nil, 403)
	nav := decodeValue[struct{ Items []Menu }](t, reader.call("GET", "/me/navigation", nil, 200))
	var walk func([]Menu)
	walk = func(items []Menu) {
		for _, m := range items {
			if m.PageKey == "files" || m.PageKey == "menus" || m.PageKey == "roles" || m.PageKey == "settings" || m.PageKey == "audit" || m.Kind == "link" {
				t.Fatal("unauthorized navigation", m)
			}
			walk(m.Children)
		}
	}
	walk(nav.Items)
}
func TestIndependentFilePermissions(t *testing.T) {
	_, ts, c := fixture(t)
	f := uploadForTest(t, c, "2", "readme.txt", []byte("preview"), 200)
	role := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]any{"name": "文件查看", "description": ""}, 201))
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"permissions": []string{"file:read"}, "version": 1}, 200)
	c.call("POST", "/users", userInput{Username: "file-reader", Name: "文件查看者", Password: "Read-password-123!", Roles: []string{role["id"]}}, 201)
	reader := client(t, ts.URL)
	reader.login("file-reader", "Read-password-123!")
	reader.call("GET", "/files", nil, 200)
	reader.call("GET", "/files/"+f.ID+"/preview", nil, 200)
	binaryForTest(t, reader, "/files/"+f.ID+"/download", 403)
	uploadForTest(t, reader, "2", "new.txt", []byte("denied"), 403)
	reader.call("DELETE", "/files/"+f.ID, map[string]int{"version": 1}, 403)
	reader.call("POST", "/image-styles", ImageStyle{Name: "不允许", Width: 32, Height: 32, Fit: "cover"}, 403)
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"permissions": []string{"file:upload"}, "version": 2}, 200)
	reader.call("GET", "/files", nil, 401)
	reader.login("file-reader", "Read-password-123!")
	reader.call("GET", "/files", nil, 200)
	reader.call("GET", "/files/"+f.ID+"/preview", nil, 403)
	uploadForTest(t, reader, "2", "allowed.txt", []byte("upload"), 200)
}
func TestV2UpgradeKeepsUsersAndGrantsAdminOnly(t *testing.T) {
	dir := t.TempDir()
	db, e := sql.Open("sqlite", filepath.Join(dir, "monitor.db"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schema + "\n" + migration2); e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO roles(id,name,builtin) VALUES(1,'管理员',1),(2,'普通成员',1); INSERT INTO role_permissions VALUES(1,'role:manage'),(2,'host:read'); INSERT INTO users(id,username,display_name,password_hash) VALUES(1,'legacy','迁移保留','hash'); INSERT INTO user_roles VALUES(1,1); INSERT INTO sessions VALUES('old',1,'csrf',1,9999999999999);`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	store, e := OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	var count int
	store.DB.QueryRow("SELECT count(*) FROM role_permissions WHERE role_id=1 AND permission IN ('menu:manage','file:read','file:upload','file:download','file:manage','file:styles','system:read','system:settings','audit:read')").Scan(&count)
	if count != 9 {
		t.Fatal("admin permissions not upgraded", count)
	}
	store.DB.QueryRow("SELECT count(*) FROM role_permissions WHERE role_id=2 AND permission LIKE 'file:%'").Scan(&count)
	if count != 0 {
		t.Fatal("privilege escalation")
	}
	store.DB.QueryRow("SELECT count(*) FROM sessions").Scan(&count)
	if count != 0 {
		t.Fatal("session survived upgrade")
	}
	var name string
	store.DB.QueryRow("SELECT display_name FROM users WHERE id=1").Scan(&name)
	if name != "迁移保留" {
		t.Fatal("existing user lost")
	}
	store.Close()
	store, e = OpenStore(dir)
	if e != nil {
		t.Fatal("migration not restart safe", e)
	}
	defer store.Close()
	store.DB.QueryRow("SELECT count(*) FROM menus").Scan(&count)
	if count != 16 {
		t.Fatal("seed duplicated", count)
	}
}

func TestUploadCrashResidueAndAuditIsolation(t *testing.T) {
	s, ts, c := fixture(t)
	f := uploadForTest(t, c, "2", "kept.txt", []byte("keep"), 200)
	stored, e := readFile(s.Store.DB, f.ID)
	if e != nil {
		t.Fatal(e)
	}
	old := time.Now().Add(-25 * time.Hour)
	paths := []string{"staging/" + randomToken(), "objects/" + randomToken()}
	for _, path := range paths {
		if e = s.Store.Files.WriteFile(path, []byte("interrupted"), 0600); e != nil {
			t.Fatal(e)
		}
		s.Store.Files.Chtimes(path, old, old)
	}
	recent := "staging/" + randomToken()
	s.Store.Files.WriteFile(recent, []byte("active"), 0600)
	s.Store.Files.Chtimes("objects/"+stored.ObjectKey, old, old)
	s.cleanupUploadResidue(context.Background())
	for _, path := range paths {
		if _, e = s.Store.Files.Stat(path); !os.IsNotExist(e) {
			t.Fatal("old orphan remained", path, e)
		}
	}
	if _, e = s.Store.Files.Stat(recent); e != nil {
		t.Fatal("recent transfer deleted")
	}
	binaryForTest(t, c, "/files/"+f.ID+"/download", 200)
	role := decodeValue[map[string]string](t, c.call("POST", "/roles", map[string]string{"name": "仅授权管理员"}, 201))
	c.call("PATCH", "/roles/"+role["id"], map[string]any{"version": 1, "permissions": []string{"role:manage"}}, 200)
	c.call("POST", "/users", userInput{Username: "role-only", Name: "仅管理角色", Password: "Read-password-123!", Roles: []string{role["id"]}}, 201)
	reader := client(t, ts.URL)
	reader.login("role-only", "Read-password-123!")
	overview := decodeValue[struct{ Activity []Activity }](t, reader.call("GET", "/overview", nil, 200))
	if len(overview.Activity) != 0 {
		t.Fatal("audit leaked through overview")
	}
	reader.call("GET", "/audit", nil, 403)
}
