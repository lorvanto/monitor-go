package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func profileInput(username, password string, version int) map[string]any {
	return map[string]any{"username": username, "currentPassword": password, "version": version}
}

func TestProfileRenamePreservesSessionGrantsAndHistory(t *testing.T) {
	s, ts, c := fixture(t)
	before := decodeValue[User](t, c.call("GET", "/me", nil, 200))
	var oldResource string
	if e := s.Store.DB.QueryRow("SELECT resource FROM audit_logs ORDER BY id LIMIT 1").Scan(&oldResource); e != nil {
		t.Fatal(e)
	}
	renamed := decodeValue[User](t, c.call("PATCH", "/me", profileInput("  New.Admin-1  ", "Test-password-123!", before.Version), 200))
	if renamed.Username != "new.admin-1" || renamed.ID != before.ID || renamed.Name != before.Name || renamed.CSRF != before.CSRF || renamed.Version != before.Version+1 || !reflect.DeepEqual(renamed.Roles, before.Roles) || !reflect.DeepEqual(renamed.Permissions, before.Permissions) {
		t.Fatalf("unexpected profile mutation: before=%+v after=%+v", before, renamed)
	}
	current := decodeValue[User](t, c.call("GET", "/me", nil, 200))
	if current.Username != renamed.Username || current.CSRF != before.CSRF {
		t.Fatal("current session did not survive rename")
	}
	other := client(t, ts.URL)
	other.call("POST", "/auth/login", map[string]string{"username": "admin", "password": "Test-password-123!"}, 401)
	other.login("NEW.ADMIN-1", "Test-password-123!")
	var resource, action string
	if e := s.Store.DB.QueryRow("SELECT resource FROM audit_logs ORDER BY id LIMIT 1").Scan(&resource); e != nil || resource != oldResource {
		t.Fatal("historical audit record was rewritten", e)
	}
	if e := s.Store.DB.QueryRow("SELECT action,resource FROM audit_logs ORDER BY id DESC LIMIT 1").Scan(&action, &resource); e != nil || action != "修改登录用户名" || resource != "admin → new.admin-1" || strings.Contains(resource, "Test-password") {
		t.Fatal("missing or unsafe rename audit", action, resource, e)
	}
}

func TestProfileSelfAuthorizationPasswordDuplicatesAndCSRF(t *testing.T) {
	s, ts, admin := fixture(t)
	created := decodeValue[map[string]string](t, admin.call("POST", "/users", userInput{Username: "reader", Name: "Reader", Password: "Read-password-123!", Roles: []string{"3"}}, 201))
	reader := client(t, ts.URL)
	reader.login("reader", "Read-password-123!")
	user := decodeValue[User](t, reader.call("GET", "/me", nil, 200))
	if user.Can("user:manage") {
		t.Fatal("test user should lack user management grants")
	}
	client(t, ts.URL).call("PATCH", "/me", profileInput("blocked", "Read-password-123!", user.Version), 401)
	csrf := reader.csrf
	reader.csrf = ""
	reader.call("PATCH", "/me", profileInput("blocked", "Read-password-123!", user.Version), 403)
	reader.csrf = csrf
	reader.call("PATCH", "/me", profileInput("renamed-reader", "wrong-password", user.Version), 422)
	reader.call("PATCH", "/me", profileInput("  ADMIN ", "Read-password-123!", user.Version), 422)
	for _, name := range []string{"ab", "_reader", "bad name", strings.Repeat("a", 41)} {
		reader.call("PATCH", "/me", profileInput(name, "Read-password-123!", user.Version), 422)
	}
	withTarget := profileInput("renamed-reader", "Read-password-123!", user.Version)
	withTarget["id"] = "1"
	reader.call("PATCH", "/me", withTarget, 422)
	reader.call("PATCH", "/me", profileInput("renamed-reader", "", user.Version), 422)
	renamed := decodeValue[User](t, reader.call("PATCH", "/me", profileInput("renamed-reader", "Read-password-123!", user.Version), 200))
	if idText(renamed.ID) != created["id"] || renamed.Username != "renamed-reader" {
		t.Fatal("self-service rename targeted the wrong account")
	}
	var adminName string
	if e := s.Store.DB.QueryRow("SELECT username FROM users WHERE id=1").Scan(&adminName); e != nil || adminName != "admin" {
		t.Fatal("self-service changed another account", e)
	}
	reader.call("PATCH", "/me", profileInput("stale-reader", "Read-password-123!", user.Version), 409)
	stillCurrent := decodeValue[User](t, reader.call("GET", "/me", nil, 200))
	if stillCurrent.Username != "renamed-reader" {
		t.Fatal("stale edit overwrote a committed rename")
	}
	var audits int
	if e := s.Store.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action='修改登录用户名'").Scan(&audits); e != nil || audits != 1 {
		t.Fatal("failed writes created audit records", audits, e)
	}
	// The fixture's Store uses a real SQLite file; verify the committed row rather
	// than relying only on the HTTP response or an in-memory substitute.
	var stored string
	if e := s.Store.DB.QueryRow("SELECT username FROM users WHERE id=?", renamed.ID).Scan(&stored); e != nil || stored != renamed.Username {
		t.Fatal("rename was not persisted", stored, e)
	}
}

func TestProfileDuplicateErrorIsLocalized(t *testing.T) {
	_, _, c := fixture(t)
	c.call("POST", "/users", userInput{Username: "occupied", Name: "Occupied", Password: "Read-password-123!", Roles: []string{"3"}}, 201)
	user := decodeValue[User](t, c.call("GET", "/me", nil, 200))
	b, _ := json.Marshal(profileInput("occupied", "Test-password-123!", user.Version))
	req, e := http.NewRequest("PATCH", c.base+"/api/v1/me", bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Set("Accept-Language", "en-US")
	res, e := c.c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var response struct {
		Error struct{ Key, Message string }
	}
	if e = json.NewDecoder(res.Body).Decode(&response); e != nil || res.StatusCode != 422 || response.Error.Key != "用户名已存在" || response.Error.Message != "Username already exists" {
		t.Fatalf("unexpected localized duplicate response: status=%d response=%+v error=%v", res.StatusCode, response, e)
	}
}

func TestProfileDisplayNameValidationAtomicUpdateAndCompatibility(t *testing.T) {
	s, _, c := fixture(t)
	before := decodeValue[User](t, c.call("GET", "/me", nil, 200))
	for _, name := range []string{"", " \t\n ", strings.Repeat("a", 81), strings.Repeat("名", 27)} {
		input := profileInput("unchanged-admin", "Test-password-123!", before.Version)
		input["name"] = name
		c.call("PATCH", "/me", input, 422)
	}
	input := profileInput("new-admin", "Test-password-123!", before.Version)
	input["name"] = "  新显示名称  "
	updated := decodeValue[User](t, c.call("PATCH", "/me", input, 200))
	if updated.Username != "new-admin" || updated.Name != "新显示名称" || updated.CSRF != before.CSRF || updated.ID != before.ID || !reflect.DeepEqual(updated.Permissions, before.Permissions) {
		t.Fatalf("unexpected combined update: %+v", updated)
	}
	var username, name string
	if e := s.Store.DB.QueryRow("SELECT username,display_name FROM users WHERE id=?", before.ID).Scan(&username, &name); e != nil || username != updated.Username || name != updated.Name {
		t.Fatal("combined profile change was not stored atomically", username, name, e)
	}
	var actor, resource string
	if e := s.Store.DB.QueryRow("SELECT actor,resource FROM audit_logs ORDER BY id LIMIT 1").Scan(&actor, &resource); e != nil || actor != before.Name || resource != "admin" {
		t.Fatal("display name update rewrote historical audit identity", actor, resource, e)
	}
	var action string
	if e := s.Store.DB.QueryRow("SELECT actor,action,resource FROM audit_logs ORDER BY id DESC LIMIT 1").Scan(&actor, &action, &resource); e != nil || actor != before.Name || action != "账号设置已保存" || resource != idText(before.ID) {
		t.Fatal("display name change did not append safe account audit", actor, action, resource, e)
	}
	input = profileInput(updated.Username, "Test-password-123!", updated.Version)
	input["name"] = "Name only"
	nameOnly := decodeValue[User](t, c.call("PATCH", "/me", input, 200))
	if nameOnly.Name != "Name only" || nameOnly.Username != updated.Username || nameOnly.CSRF != before.CSRF {
		t.Fatal("name-only edit changed login or session", nameOnly)
	}
	// Older clients omit name; that must retain the currently saved display name.
	compatible := decodeValue[User](t, c.call("PATCH", "/me", profileInput("username-only", "Test-password-123!", nameOnly.Version), 200))
	if compatible.Name != "Name only" || compatible.Username != "username-only" {
		t.Fatal("username-only request cleared display name", compatible)
	}
	current := decodeValue[User](t, c.call("GET", "/me", nil, 200))
	if current.Name != compatible.Name || current.CSRF != before.CSRF {
		t.Fatal("existing session did not see updated name", current)
	}
}
