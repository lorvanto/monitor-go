package app

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/lorvanto/monitor-go/locales"
)

func TestEveryAPIErrorHasEnglishTranslation(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		check := func(expr ast.Expr) {
			literal, ok := expr.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return
			}
			key, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.ContainsFunc(locales.Text("en-US", key), func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
				t.Errorf("%s: missing English API message: %s", fset.Position(expr.Pos()), key)
			}
		}
		ast.Inspect(f, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				if id, ok := n.Fun.(*ast.Ident); ok && (id.Name == "bad" || id.Name == "badf") && len(n.Args) > 0 {
					check(n.Args[0])
				}
				if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "auditRequest" && len(n.Args) > 3 {
					ast.Inspect(n.Args[3], func(child ast.Node) bool {
						if expr, ok := child.(ast.Expr); ok {
							check(expr)
						}
						return true
					})
				}
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && (id.Name == "action" || id.Name == "message") && i < len(n.Rhs) {
						check(n.Rhs[i])
					}
				}
			case *ast.CompositeLit:
				if id, ok := n.Type.(*ast.Ident); ok && id.Name == "apiError" {
					for _, value := range n.Elts {
						if kv, ok := value.(*ast.KeyValueExpr); ok {
							check(kv.Value)
						} else {
							check(value)
						}
					}
				}
			}
			return true
		})
	}
}

func TestUIErrorHTTPContract(t *testing.T) {
	for _, language := range locales.Supported {
		t.Run(language, func(t *testing.T) { testUIErrorHTTPContract(t, language) })
	}
}

func testUIErrorHTTPContract(t *testing.T, language string) {
	s, ts, c := fixture(t)
	for _, tc := range []struct {
		method, path, body, key string
		status                  int
	}{
		{"POST", "/auth/login", `{"username":"missing","password":"incorrect"}`, "用户名或密码不正确", 401},
		{"GET", "/missing-route", "", "接口不存在", 404},
		{"POST", "/roles", `{}`, "请填写有效的角色名称和说明", 422},
		{"POST", "/users", `{}`, "姓名不能为空或超过 80 字节", 422},
		{"POST", "/applications", `{}`, "请填写名称、3–48 位小写应用标识及不超过 1000 字节的说明", 422},
		{"POST", "/notification-channels", `{}`, "通知渠道名称长度为 1–120 字节", 422},
		{"POST", "/image-styles", `{}`, "请输入名称、16–2048 像素尺寸以及有效缩放方式", 422},
		{"POST", "/files/folders", `{"name":"","parentId":"1"}`, "名称为 1–200 字节，不能包含路径分隔符或控制字符", 422},
		{"POST", "/monitors", `{}`, "检测名称、地址或分组长度不正确", 422},
		{"POST", "/menus", `{}`, "菜单名称长度为 1–120 字节", 422},
		{"GET", "/files/999999/download?lang=" + language, "", "记录不存在", 404},
		{"GET", "/releases/999999/download?lang=" + language, "", "记录不存在", 404},
		{"POST", "/tools/codes", `{"kind":"ean13","content":"1","width":512,"height":160,"level":"M"}`, "请输入 %d 位数字（自动校验）或 %d 位完整编码", 422},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, ts.URL+"/api/v1"+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-CSRF-Token", c.csrf)
			req.Header.Set("Accept-Language", language)
			if strings.Contains(tc.path, "?lang=") {
				req.Header.Set("Accept-Language", "zh-CN")
			}
			res, err := c.c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			raw, _ := io.ReadAll(res.Body)
			var body struct {
				Error struct {
					Message, Key string
					Args         []any
				}
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			expected := locales.Text(language, tc.key)
			if len(body.Error.Args) > 0 {
				expected = strings.ReplaceAll(expected, "%d", "%v")
				expected = locales.Format(language, expected, body.Error.Args...)
			}
			if res.StatusCode != tc.status || res.Header.Get("Content-Language") != language || body.Error.Key != tc.key || body.Error.Message != expected {
				t.Fatalf("status=%d body=%s expected=%s", res.StatusCode, raw, expected)
			}
		})
	}
	// Setup errors on a fresh, real SQLite installation.
	fresh, err := New(Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Store.Close()
	setup := httptest.NewServer(fresh.Handler())
	defer setup.Close()
	req, _ := http.NewRequest("POST", setup.URL+"/api/v1/setup", strings.NewReader(`{"token":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", language)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 403 || !bytes.Contains(raw, []byte(locales.Text(language, "初始化码不正确"))) {
		t.Fatalf("setup: %d %s", res.StatusCode, raw)
	}
	if _, err := os.Stat(filepath.Join(fresh.Store.Dir, "setup-token")); err != nil {
		t.Fatal("invalid setup consumed token", err)
	}
	// An internal error stays generic and does not expose the underlying SQL error.
	mustSQL(t, s, "DROP TABLE applications")
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/applications", nil)
	req.Header.Set("Accept-Language", language)
	res, err = c.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 500 || !bytes.Contains(raw, []byte(locales.Text(language, "操作失败，请稍后重试"))) || bytes.Contains(raw, []byte("no such table")) {
		t.Fatalf("500: %d %s", res.StatusCode, raw)
	}
}

func TestDefaultMonitorGroupDoesNotPersistDisplayText(t *testing.T) {
	s, _, c := fixture(t)
	for _, group := range []string{"", "默认分组", "Custom"} {
		in := monitorPayload("https://example.com")
		in.Group = group
		result := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))
		id := result["id"]
		var stored string
		if err := s.Store.DB.QueryRow("SELECT group_name FROM monitors WHERE id=?", id).Scan(&stored); err != nil || stored != group {
			t.Fatalf("stored group %q: %v", stored, err)
		}
		mustSQL(t, s, "INSERT INTO check_results(monitor_id,at,state,latency,detail) VALUES(?,?,'healthy',10,'')", id, now()-1000)
	}
	for _, hours := range []string{"1", "720", "2160"} {
		for _, filter := range []struct {
			query string
			want  int
		}{{"", 3}, {"&group=", 3}, {"&ungrouped=1", 1}, {"&group=Custom", 1}} {
			report := decodeValue[struct{ Samples int }](t, c.call("GET", "/overview?hours="+hours+filter.query, nil, 200))
			if report.Samples != filter.want {
				t.Fatalf("%s%s: %d", hours, filter.query, report.Samples)
			}
		}
	}
}
