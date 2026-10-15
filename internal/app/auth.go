package app

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,39}$`)

func validPassword(p string) bool { return len(p) >= 12 && len(p) <= 72 }
func (s *Server) identity(r *http.Request) (*User, error) {
	cookie, e := r.Cookie("monitor_session")
	if e != nil {
		return nil, &apiError{401, "请先登录"}
	}
	u := &User{}
	e = s.Store.DB.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.display_name,u.enabled,u.version,s.csrf FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND s.auth_version=u.auth_version AND u.enabled=1`, digest(cookie.Value), now()).Scan(&u.ID, &u.Username, &u.Name, &u.Enabled, &u.Version, &u.CSRF)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, &apiError{401, "登录已失效，请重新登录"}
	}
	if e != nil {
		return nil, e
	}
	u.Permissions = []string{}
	u.Roles = []string{}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT DISTINCT p.permission FROM user_roles ur JOIN role_permissions p ON p.role_id=ur.role_id WHERE ur.user_id=?`, u.ID)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var p string
		if e = rows.Scan(&p); e != nil {
			rows.Close()
			return nil, e
		}
		u.Permissions = append(u.Permissions, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	rows, e = s.Store.DB.QueryContext(r.Context(), "SELECT role_id FROM user_roles WHERE user_id=?", u.ID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		u.Roles = append(u.Roles, idText(id))
	}
	return u, rows.Err()
}
func (s *Server) session(w http.ResponseWriter, id int64) (any, error) {
	token := randomToken()
	csrf := randomToken()
	expires := time.Now().Add(12 * time.Hour)
	_, e := s.Store.DB.Exec(`INSERT INTO sessions(token_hash,user_id,csrf,auth_version,expires_at) SELECT ?,id,?,auth_version,? FROM users WHERE id=?`, digest(token), csrf, expires.UnixMilli(), id)
	if e != nil {
		return nil, e
	}
	http.SetCookie(w, &http.Cookie{Name: "monitor_session", Value: token, Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookie, SameSite: http.SameSiteStrictMode, Expires: expires})
	return map[string]any{"csrf": csrf}, nil
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !s.allowedLogin(r) {
		return nil, &apiError{429, "请求过于频繁，请 5 分钟后再试"}
	}
	var in struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	if s.setupToken == "" || subtle.ConstantTimeCompare([]byte(in.Token), []byte(s.setupToken)) != 1 {
		return nil, &apiError{403, "初始化码不正确"}
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if !usernamePattern.MatchString(in.Username) || !validPassword(in.Password) || len(strings.TrimSpace(in.Name)) < 1 || len(in.Name) > 80 {
		return nil, bad("用户名需要 3–40 位英文数字，密码需要 12–72 字节，姓名不能为空")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if e != nil {
		return nil, e
	}
	var id int64
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var n int
		if e := tx.QueryRow("SELECT count(*) FROM users").Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return &apiError{409, "系统已完成初始化"}
		}
		res, e := tx.Exec("INSERT INTO users(username,display_name,password_hash) VALUES(?,?,?)", in.Username, strings.TrimSpace(in.Name), string(hash))
		if e != nil {
			return e
		}
		id, _ = res.LastInsertId()
		if _, e = tx.Exec("INSERT INTO user_roles VALUES(?,1)", id); e != nil {
			return e
		}
		return auditRequest(tx, r, in.Name, "创建首个管理员", in.Username)
	})
	if e != nil {
		return nil, e
	}
	os.Remove(filepath.Join(s.Store.Dir, "setup-token"))
	return s.session(w, id)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !s.allowedLogin(r) {
		return nil, &apiError{429, "登录尝试过多，请 5 分钟后再试"}
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	var id int64
	var hash string
	e := s.Store.DB.QueryRowContext(r.Context(), "SELECT id,password_hash FROM users WHERE username=? AND enabled=1", strings.ToLower(strings.TrimSpace(in.Username))).Scan(&id, &hash)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	if e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		return nil, &apiError{401, "用户名或密码不正确"}
	}
	return s.session(w, id)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, _ := r.Cookie("monitor_session")
	_, e := s.Store.DB.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", digest(c.Value))
	http.SetCookie(w, &http.Cookie{Name: "monitor_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.SecureCookie, SameSite: http.SameSiteStrictMode})
	return true, e
}
