package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

type Application struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Version     int    `json:"version"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	ActiveKeys  int    `json:"activeKeys"`
	Releases    int    `json:"releases"`
}

const appColumns = "a.id,a.slug,a.name,a.description,a.enabled,a.version,a.created_at,a.updated_at,(SELECT count(*) FROM api_keys k WHERE k.application_id=a.id AND k.revoked_at IS NULL AND k.expires_at>?), (SELECT count(*) FROM app_releases p WHERE p.application_id=a.id AND p.deleted_at IS NULL)"

func scanApp(row scanner) (Application, error) {
	var a Application
	e := row.Scan(&a.ID, &a.Slug, &a.Name, &a.Description, &a.Enabled, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.ActiveKeys, &a.Releases)
	if e == sql.ErrNoRows {
		e = missing()
	}
	return a, e
}
func readApplication(q rowQuery, id any) (Application, error) {
	return scanApp(q.QueryRow("SELECT "+appColumns+" FROM applications a WHERE a.id=? AND a.deleted_at IS NULL", now(), id))
}
func (s *Server) listApplications(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT "+appColumns+" FROM applications a WHERE a.deleted_at IS NULL ORDER BY a.id DESC", now())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []Application{}
	for rows.Next() {
		a, e := scanApp(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

var appSlug = regexp.MustCompile(`^[a-z][a-z0-9-]{2,47}$`)

func (s *Server) saveApplication(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
		Version     int    `json:"version"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if !appSlug.MatchString(in.Slug) || in.Name == "" || len(in.Name) > 120 || len(in.Description) > 1000 {
		return nil, bad("请填写名称、3–48 位小写应用标识及不超过 1000 字节的说明")
	}
	var id int64
	var e error
	if r.Method == "PATCH" {
		id, e = pathID(r)
		if e != nil {
			return nil, e
		}
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		if id > 0 {
			a, e := readApplication(tx, id)
			if e != nil {
				return e
			}
			if a.Version != in.Version {
				return conflict()
			}
			if a.Slug != in.Slug {
				return bad("应用标识创建后不可修改")
			}
			_, e = tx.Exec("UPDATE applications SET name=?,description=?,enabled=?,version=version+1,updated_at=? WHERE id=?", in.Name, in.Description, in.Enabled, now(), id)
			if e != nil {
				return e
			}
		} else {
			var n int
			if e := tx.QueryRow("SELECT count(*) FROM applications WHERE deleted_at IS NULL").Scan(&n); e != nil {
				return e
			}
			if n >= 200 {
				return bad("最多配置 200 个有效应用")
			}
			if e := tx.QueryRow("SELECT count(*) FROM applications WHERE slug=?", in.Slug).Scan(&n); e != nil {
				return e
			}
			if n > 0 {
				return bad("应用标识已被使用（包含已归档应用）")
			}
			res, e := tx.Exec("INSERT INTO applications(slug,name,description,enabled,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", in.Slug, in.Name, in.Description, in.Enabled, u.ID, now(), now())
			if e != nil {
				return e
			}
			id, _ = res.LastInsertId()
		}
		return auditRequest(tx, r, u.Name, "保存应用", in.Name)
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) archiveApplication(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		a, e := readApplication(tx, id)
		if e != nil {
			return e
		}
		if a.Version != in.Version {
			return conflict()
		}
		if a.Releases > 0 {
			return bad("请先归档该应用下的所有安装包")
		}
		if _, e = tx.Exec("UPDATE applications SET deleted_at=?,enabled=0,version=version+1 WHERE id=?", now(), id); e != nil {
			return e
		}
		if _, e = tx.Exec("UPDATE api_keys SET revoked_at=?,version=version+1 WHERE application_id=? AND revoked_at IS NULL", now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "归档应用并撤销密钥", a.Name)
	})
	return true, e
}

type KeyScope struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var keyScopes = []KeyScope{{"host:read", "主机状态", "读取本实例内全部主机的状态与资源指标"}, {"monitor:read", "检测状态", "读取全部检测目标和最近状态，不包含访问凭据"}, {"alert:read", "告警事件", "读取全部监控告警"}, {"release:read", "安装包列表", "仅查看当前应用已发布的安装包"}, {"release:download", "安装包下载", "仅下载当前应用已发布的安装包"}}

type APIKey struct {
	ID            string   `json:"id"`
	ApplicationID string   `json:"applicationId"`
	Name          string   `json:"name"`
	Prefix        string   `json:"prefix"`
	Scopes        []string `json:"scopes"`
	ExpiresAt     int64    `json:"expiresAt"`
	LastUsedAt    int64    `json:"lastUsedAt"`
	CreatedAt     int64    `json:"createdAt"`
	RevokedAt     int64    `json:"revokedAt"`
	Version       int      `json:"version"`
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	if _, e = readApplication(s.Store.DB, id); e != nil {
		return nil, e
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT id,application_id,name,key_prefix,scopes,expires_at,coalesce(last_used_at,0),created_at,coalesce(revoked_at,0),version FROM api_keys WHERE application_id=? ORDER BY id DESC LIMIT 100", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		var k APIKey
		var scopes string
		if e = rows.Scan(&k.ID, &k.ApplicationID, &k.Name, &k.Prefix, &scopes, &k.ExpiresAt, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt, &k.Version); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(scopes), &k.Scopes); e != nil {
			return nil, e
		}
		out = append(out, k)
	}
	return map[string]any{"items": out, "scopes": keyScopes}, rows.Err()
}
func (s *Server) createKey(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Name     string   `json:"name"`
		Scopes   []string `json:"scopes"`
		Days     int      `json:"days"`
		RotateID string   `json:"rotateId"`
		Version  int      `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || in.Days < 1 || in.Days > 365 || len(in.Scopes) < 1 || len(in.Scopes) > len(keyScopes) {
		return nil, bad("请输入名称、1–365 天有效期并选择授权范围")
	}
	slices.Sort(in.Scopes)
	in.Scopes = slices.Compact(in.Scopes)
	for _, key := range in.Scopes {
		if !slices.ContainsFunc(keyScopes, func(s KeyScope) bool { return s.Key == key }) {
			return nil, bad("密钥包含不支持的权限范围")
		}
	}
	token := "ymk_" + randomToken()
	prefix := token[:12]
	scopes, _ := json.Marshal(in.Scopes)
	expiry := now() + int64(in.Days)*86400000
	var keyID int64
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		a, e := readApplication(tx, id)
		if e != nil {
			return e
		}
		if !a.Enabled {
			return bad("请先启用应用")
		}
		if in.RotateID != "" {
			res, e := tx.Exec("UPDATE api_keys SET revoked_at=?,version=version+1 WHERE id=? AND application_id=? AND version=? AND revoked_at IS NULL", now(), in.RotateID, id, in.Version)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return conflict()
			}
		}
		var n int
		if e = tx.QueryRow("SELECT count(*) FROM api_keys WHERE application_id=? AND revoked_at IS NULL AND expires_at>?", id, now()).Scan(&n); e != nil {
			return e
		}
		if n >= 10 {
			return bad("每个应用最多同时拥有 10 个有效密钥")
		}
		res, e := tx.Exec("INSERT INTO api_keys(application_id,name,key_prefix,secret_hash,scopes,expires_at,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)", id, in.Name, prefix, digest(token), string(scopes), expiry, u.ID, now())
		if e != nil {
			return e
		}
		keyID, _ = res.LastInsertId()
		action := "创建应用密钥"
		if in.RotateID != "" {
			action = "轮换应用密钥"
		}
		return auditRequest(tx, r, u.Name, action, a.Name+" / "+in.Name)
	})
	if e != nil {
		return nil, e
	}
	return map[string]any{"id": idText(keyID), "token": token, "prefix": prefix, "expiresAt": expiry}, nil
}
func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		res, e := tx.Exec("UPDATE api_keys SET revoked_at=?,version=version+1 WHERE id=? AND version=? AND revoked_at IS NULL", now(), id, in.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict()
		}
		return auditRequest(tx, r, u.Name, "撤销应用密钥", idText(id))
	})
	return true, e
}

type machineIdentity struct {
	KeyID         string   `json:"keyId"`
	ApplicationID string   `json:"applicationId"`
	Application   string   `json:"application"`
	Scopes        []string `json:"scopes"`
	ExpiresAt     int64    `json:"expiresAt"`
}
type machineHandler func(http.ResponseWriter, *http.Request, machineIdentity) (any, error)

func (s *Server) machine(scope string, fn machineHandler) handler {
	return func(w http.ResponseWriter, r *http.Request, _ *User) (any, error) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		invalid := &apiError{401, "应用密钥无效、已过期、已撤销或应用已停用"}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || !strings.HasPrefix(token, "ymk_") || len(token) != 68 || r.URL.Query().Has("api_key") || r.URL.Query().Has("token") {
			return nil, invalid
		}
		var v machineIdentity
		var scopes string
		e := s.Store.DB.QueryRowContext(r.Context(), "SELECT k.id,a.id,a.name,k.scopes,k.expires_at FROM api_keys k JOIN applications a ON a.id=k.application_id WHERE k.secret_hash=? AND k.revoked_at IS NULL AND k.expires_at>? AND a.enabled=1 AND a.deleted_at IS NULL", digest(token), now()).Scan(&v.KeyID, &v.ApplicationID, &v.Application, &scopes, &v.ExpiresAt)
		if e == sql.ErrNoRows {
			return nil, invalid
		}
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(scopes), &v.Scopes); e != nil {
			return nil, e
		}
		if scope != "" && !slices.Contains(v.Scopes, scope) {
			return nil, &apiError{403, "应用密钥没有此接口的授权范围"}
		}
		if !s.allowedRequest(r, "app-key:"+v.KeyID+":", 300) {
			w.Header().Set("Retry-After", "300")
			return nil, &apiError{429, "应用请求过于频繁，请稍后重试"}
		}
		if _, e = s.Store.DB.ExecContext(r.Context(), "UPDATE api_keys SET last_used_at=? WHERE id=? AND coalesce(last_used_at,0)<?", now(), v.KeyID, now()-60000); e != nil {
			return nil, e
		}
		return fn(w, r, v)
	}
}
func (s *Server) machineHosts(w http.ResponseWriter, r *http.Request, key machineIdentity) (any, error) {
	hs, e := s.hosts(r.Context())
	if e != nil {
		return nil, e
	}
	return hs, nil
}
func (s *Server) machineMonitors(w http.ResponseWriter, r *http.Request, key machineIdentity) (any, error) {
	// A narrow projection deliberately excludes usernames, encrypted credentials and response details.
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT id,name,type,target,group_name,enabled,state,coalesce(checked_at,0) FROM monitors WHERE deleted_at IS NULL ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, kind, target, group, state string
		var enabled bool
		var checked int64
		if e = rows.Scan(&id, &name, &kind, &target, &group, &enabled, &state, &checked); e != nil {
			return nil, e
		}
		items = append(items, map[string]any{"id": id, "name": name, "type": kind, "target": target, "group": group, "enabled": enabled, "status": state, "checkedAt": checked})
	}
	return items, rows.Err()
}
func (s *Server) machineAlerts(w http.ResponseWriter, r *http.Request, key machineIdentity) (any, error) {
	return s.listAlerts(w, r, &User{Permissions: []string{"alert:read"}})
}
