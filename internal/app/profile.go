package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// updateProfile deliberately addresses the authenticated user only. Renaming an
// account does not change its ID, grants, authentication version, or sessions.
func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Username        string  `json:"username"`
		Name            *string `json:"name"`
		CurrentPassword string  `json:"currentPassword"`
		Version         int     `json:"version"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if !usernamePattern.MatchString(in.Username) {
		return nil, bad("用户名需为 3–40 位小写英文字母、数字、下划线、点或连字符，且以字母或数字开头。")
	}
	name := u.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if len(name) < 1 || len(name) > 80 {
			return nil, bad("姓名不能为空或超过 80 字节")
		}
	}
	if in.CurrentPassword == "" {
		return nil, bad("请填写当前密码")
	}
	if in.Version != u.Version {
		return nil, conflict()
	}
	finish, allowed := s.beginLogin(r, u.Username)
	if !allowed {
		return nil, &apiError{429, "登录尝试过多，请 5 分钟后再试"}
	}
	failed := false
	defer func() { finish(failed) }()
	var hash string
	e := s.Store.DB.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=? AND enabled=1 AND version=?", u.ID, in.Version).Scan(&hash)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, conflict()
	}
	if e != nil {
		return nil, e
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.CurrentPassword)) != nil {
		failed = true
		return nil, bad("当前密码不正确")
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var count int
		if e := tx.QueryRow("SELECT count(*) FROM users WHERE username=? AND id<>?", in.Username, u.ID).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return bad("用户名已存在")
		}
		res, e := tx.Exec("UPDATE users SET username=?,display_name=?,version=version+1 WHERE id=? AND enabled=1 AND version=? AND password_hash=?", in.Username, name, u.ID, in.Version, hash)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return conflict()
		}
		if name != u.Name {
			return auditRequest(tx, r, u.Name, "账号设置已保存", idText(u.ID))
		}
		return auditRequest(tx, r, u.Name, "修改登录用户名", u.Username+" → "+in.Username)
	})
	if e != nil {
		return nil, e
	}
	return s.identity(r)
}
