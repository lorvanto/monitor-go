package app

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"slices"
	"strings"
)

func (s *Server) roles() ([]Role, error) {
	out := []Role{}
	rows, e := s.Store.DB.Query("SELECT r.id,r.name,r.description,r.builtin,r.version,(SELECT count(*) FROM user_roles WHERE role_id=r.id) FROM roles r ORDER BY r.id")
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		r := Role{Permissions: []string{}}
		if e = rows.Scan(&r.ID, &r.Name, &r.Description, &r.Builtin, &r.Version, &r.Members); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		rows, e = s.Store.DB.Query("SELECT permission FROM role_permissions WHERE role_id=? ORDER BY permission", out[i].ID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var p string
			if e = rows.Scan(&p); e != nil {
				rows.Close()
				return nil, e
			}
			out[i].Permissions = append(out[i].Permissions, p)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *Server) listRoles(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return s.roles()
}
func (s *Server) createRole(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) == 0 || len(in.Name) > 80 || len(in.Description) > 500 {
		return nil, bad("请填写有效的角色名称和说明")
	}
	var id int64
	e := s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var n int
		if e := tx.QueryRow("SELECT count(*) FROM roles WHERE name=?", in.Name).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return bad("角色名称已存在")
		}
		res, e := tx.Exec("INSERT INTO roles(name,description) VALUES(?,?)", in.Name, in.Description)
		if e != nil {
			return e
		}
		id, _ = res.LastInsertId()
		return auditRequest(tx, r, u.Name, "创建角色", in.Name)
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) updateRole(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version     int      `json:"version"`
		Permissions []string `json:"permissions"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	valid := allPermissions()
	for _, p := range in.Permissions {
		if !slices.Contains(valid, p) {
			return nil, bad("包含未注册权限")
		}
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var version int
		var builtin bool
		if e := tx.QueryRow("SELECT version,builtin FROM roles WHERE id=?", id).Scan(&version, &builtin); e == sql.ErrNoRows {
			return missing()
		} else if e != nil {
			return e
		}
		if builtin {
			return bad("内置超级管理员角色不可修改")
		}
		if version != in.Version {
			return conflict()
		}
		if _, e := tx.Exec("DELETE FROM role_permissions WHERE role_id=?", id); e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, p := range in.Permissions {
			if seen[p] {
				continue
			}
			seen[p] = true
			if _, e := tx.Exec("INSERT INTO role_permissions VALUES(?,?)", id, p); e != nil {
				return e
			}
		}
		if _, e := tx.Exec("UPDATE roles SET version=version+1 WHERE id=?", id); e != nil {
			return e
		}
		if _, e := tx.Exec("DELETE FROM sessions WHERE user_id IN (SELECT user_id FROM user_roles WHERE role_id=?)", id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "更新角色权限", idText(id))
	})
	return true, e
}
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	out := []User{}
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT id,username,display_name,enabled,version FROM users ORDER BY id")
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		v := User{Roles: []string{}, Permissions: []string{}}
		if e = rows.Scan(&v.ID, &v.Username, &v.Name, &v.Enabled, &v.Version); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		rows, e = s.Store.DB.QueryContext(r.Context(), "SELECT role_id FROM user_roles WHERE user_id=?", out[i].ID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id int64
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return nil, e
			}
			out[i].Roles = append(out[i].Roles, idText(id))
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

type userInput struct {
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
	Enabled  bool     `json:"enabled"`
	Version  int      `json:"version"`
}

func validateUser(in *userInput, create bool) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if len(in.Name) < 1 || len(in.Name) > 80 {
		return bad("姓名不能为空或超过 80 字节")
	}
	if create && !usernamePattern.MatchString(in.Username) {
		return bad("用户名需要 3–40 位英文数字、下划线或点")
	}
	if (create || in.Password != "") && !validPassword(in.Password) {
		return bad("密码需要 12–72 字节")
	}
	if len(in.Roles) == 0 || len(in.Roles) > 50 {
		return bad("请至少选择一个角色")
	}
	return nil
}
func assignRoles(tx *sql.Tx, id int64, roles []string) error {
	if _, e := tx.Exec("DELETE FROM user_roles WHERE user_id=?", id); e != nil {
		return e
	}
	for _, role := range roles {
		var n int
		if e := tx.QueryRow("SELECT count(*) FROM roles WHERE id=?", role).Scan(&n); e != nil {
			return e
		}
		if n == 0 {
			return bad("角色不存在")
		}
		if _, e := tx.Exec("INSERT OR IGNORE INTO user_roles VALUES(?,?)", id, role); e != nil {
			return e
		}
	}
	return nil
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in userInput
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	if e := validateUser(&in, true); e != nil {
		return nil, e
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if e != nil {
		return nil, e
	}
	var id int64
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		var n int
		if e := tx.QueryRow("SELECT count(*) FROM users WHERE username=?", in.Username).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return bad("用户名已存在")
		}
		res, e := tx.Exec("INSERT INTO users(username,display_name,password_hash) VALUES(?,?,?)", in.Username, in.Name, string(hash))
		if e != nil {
			return e
		}
		id, _ = res.LastInsertId()
		if e = assignRoles(tx, id, in.Roles); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "创建团队成员", in.Username)
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in userInput
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	if e = validateUser(&in, false); e != nil {
		return nil, e
	}
	hash := ""
	if in.Password != "" {
		b, e := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
		if e != nil {
			return nil, e
		}
		hash = string(b)
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		res, e := tx.Exec("UPDATE users SET display_name=?,enabled=?,password_hash=CASE WHEN ?='' THEN password_hash ELSE ? END,version=version+1,auth_version=auth_version+1 WHERE id=? AND version=?", in.Name, in.Enabled, hash, hash, id, in.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict()
		}
		if e = assignRoles(tx, id, in.Roles); e != nil {
			return e
		}
		var admins int
		if e = tx.QueryRow("SELECT count(*) FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.enabled=1 AND ur.role_id=1").Scan(&admins); e != nil {
			return e
		}
		if admins < 1 {
			return bad("必须保留至少一名启用的超级管理员")
		}
		if _, e = tx.Exec("DELETE FROM sessions WHERE user_id=?", id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "更新团队成员", idText(id))
	})
	return true, e
}
