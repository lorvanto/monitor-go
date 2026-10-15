package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type PageDefinition struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	Permission string `json:"permission"`
	Icon       string `json:"icon"`
}

var pages = []PageDefinition{
	{"applications", "应用与密钥", "application:access", "KeyRound"}, {"releases", "安装包管理", "release:access", "Package"}, {"codes", "编码工具", "tool:generate", "QrCode"},
	{"overview", "监控总览", "", "LayoutDashboard"}, {"host", "主机详情", "host:read", "Server"}, {"monitors", "服务检测", "monitor:read", "Radar"}, {"alerts", "告警中心", "alert:access", "Bell"}, {"files", "文件管理", "file:access", "FolderOpen"}, {"roles", "角色与权限", "team:read", "ShieldCheck"}, {"menus", "菜单管理", "menu:manage", "ListTree"}, {"audit", "操作审计", "audit:read", "ScrollText"}, {"settings", "系统设置", "system:access", "Settings2"},
}
var menuIcons = []string{"KeyRound", "Package", "QrCode", "Barcode", "LayoutDashboard", "Activity", "Server", "Radar", "Bell", "FolderOpen", "FileText", "Image", "ShieldCheck", "ListTree", "ScrollText", "Settings2", "Globe", "Link", "Box", "Users", "Wrench", "HardDrive", "BookOpen"}

type Menu struct {
	NameKey    string `json:"nameKey,omitempty"`
	ID         int64  `json:"id,string"`
	ParentID   string `json:"parentId"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	PageKey    string `json:"pageKey"`
	URL        string `json:"url"`
	Icon       string `json:"icon"`
	Sort       int    `json:"sort"`
	Visible    bool   `json:"visible"`
	Permission string `json:"permission"`
	Builtin    bool   `json:"builtin"`
	Version    int    `json:"version"`
	Children   []Menu `json:"children,omitempty"`
}
type menuInput struct {
	ParentID   string `json:"parentId"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	PageKey    string `json:"pageKey"`
	URL        string `json:"url"`
	Icon       string `json:"icon"`
	Sort       int    `json:"sort"`
	Visible    bool   `json:"visible"`
	Permission string `json:"permission"`
	Version    int    `json:"version"`
}
type rowsQuery interface {
	Query(string, ...any) (*sql.Rows, error)
}

func readMenus(q rowsQuery) ([]Menu, error) {
	rows, e := q.Query("SELECT id,coalesce(CAST(parent_id AS TEXT),''),name,kind,page_key,external_url,icon,sort,visible,permission,builtin,version,builtin_key FROM menus WHERE deleted_at IS NULL ORDER BY sort,id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Menu{}
	for rows.Next() {
		var m Menu
		var builtinKey string
		if e = rows.Scan(&m.ID, &m.ParentID, &m.Name, &m.Kind, &m.PageKey, &m.URL, &m.Icon, &m.Sort, &m.Visible, &m.Permission, &m.Builtin, &m.Version, &builtinKey); e != nil {
			return nil, e
		}
		m.NameKey = menuNameKey(m, builtinKey)
		out = append(out, m)
	}
	return out, rows.Err()
}
func menuRevision(ms []Menu) string { b, _ := json.Marshal(ms); return digest(string(b)) }
func (s *Server) listMenus(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	ms, e := readMenus(s.Store.DB)
	return map[string]any{"items": ms, "pages": pages, "icons": menuIcons, "permissions": permissions, "revision": menuRevision(ms)}, e
}
func (s *Server) navigation(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	ms, e := readMenus(s.Store.DB)
	if e != nil {
		return nil, e
	}
	var walk func(string) []Menu
	walk = func(parent string) []Menu {
		out := []Menu{}
		for _, m := range ms {
			if m.ParentID != parent || !m.Visible || (m.Permission != "" && !u.Can(m.Permission)) {
				continue
			}
			if m.Kind == "group" {
				m.Children = walk(idText(m.ID))
				if len(m.Children) == 0 {
					continue
				}
			} else if m.Kind == "page" {
				found := false
				for _, p := range pages {
					if p.Key == m.PageKey && (p.Permission == "" || u.Can(p.Permission)) {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			out = append(out, m)
		}
		return out
	}
	var name string
	var version int
	e = s.Store.DB.QueryRowContext(r.Context(), "SELECT instance_name,version FROM system_settings WHERE id=1").Scan(&name, &version)
	nameKey := ""
	if defaultInstanceName(name, version) {
		nameKey = "我的监控中心"
		name = ""
	}
	return map[string]any{"items": walk(""), "instanceName": name, "instanceNameKey": nameKey}, e
}
func validateMenu(in *menuInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 {
		return bad("菜单名称长度为 1–120 字节")
	}
	if !slices.Contains(menuIcons, in.Icon) {
		return bad("请选择已注册的图标")
	}
	if in.Sort < 0 || in.Sort > 9999 {
		return bad("排序值范围为 0–9999")
	}
	if in.Permission != "" && !slices.Contains(allPermissions(), in.Permission) {
		return bad("访问限制必须选择已注册的权限")
	}
	switch in.Kind {
	case "group":
		in.PageKey = ""
		in.URL = ""
	case "page":
		if !slices.ContainsFunc(pages, func(p PageDefinition) bool { return p.Key == in.PageKey }) {
			return bad("请选择已实现的内部页面")
		}
		in.URL = ""
	case "link":
		v, e := url.Parse(in.URL)
		if e != nil || (v.Scheme != "http" && v.Scheme != "https") || v.Hostname() == "" || v.User != nil || len(in.URL) > 2048 {
			return bad("外链必须是有效的 HTTP 或 HTTPS 地址，且不含登录凭据")
		}
		in.PageKey = ""
	default:
		return bad("菜单类型不正确")
	}
	return nil
}
func validateMenuTree(ms []Menu) error {
	nodes := map[string]Menu{}
	for _, m := range ms {
		nodes[idText(m.ID)] = m
	}
	for _, m := range ms {
		visited := map[string]bool{idText(m.ID): true}
		parent := m.ParentID
		depth := 1
		for parent != "" {
			p, ok := nodes[parent]
			if !ok || p.Kind != "group" {
				return bad("上级必须为有效菜单分组")
			}
			if visited[parent] {
				return bad("菜单不能移动到自己或子级之下")
			}
			visited[parent] = true
			depth++
			if depth > 3 {
				return bad("菜单最多支持 3 层")
			}
			parent = p.ParentID
		}
	}
	return nil
}
func (s *Server) saveMenu(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in menuInput
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	if e := validateMenu(&in); e != nil {
		return nil, e
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
		ms, e := readMenus(tx)
		if e != nil {
			return e
		}
		builtin := false
		if id > 0 {
			index := slices.IndexFunc(ms, func(m Menu) bool { return m.ID == id })
			if index < 0 {
				return missing()
			}
			old := ms[index]
			if old.Version != in.Version {
				return conflict()
			}
			builtin = old.Builtin
			if builtin && (in.Kind != old.Kind || in.PageKey != old.PageKey) {
				return bad("内置菜单的类型与页面绑定不可修改")
			}
			ms = slices.Delete(ms, index, index+1)
		} else {
			if len(ms) >= 200 {
				return bad("最多配置 200 个菜单")
			}
			if e = tx.QueryRow("SELECT coalesce(max(id),0)+1 FROM menus").Scan(&id); e != nil {
				return e
			}
		}
		m := Menu{ID: id, ParentID: in.ParentID, Name: in.Name, Kind: in.Kind, PageKey: in.PageKey, URL: in.URL, Icon: in.Icon, Sort: in.Sort, Visible: in.Visible, Permission: in.Permission, Builtin: builtin}
		ms = append(ms, m)
		if e = validateMenuTree(ms); e != nil {
			return e
		}
		var parent any
		if in.ParentID != "" {
			parent = in.ParentID
		}
		if r.Method == "PATCH" {
			_, e = tx.Exec("UPDATE menus SET parent_id=?,name=?,kind=?,page_key=?,external_url=?,icon=?,sort=?,visible=?,permission=?,version=version+1 WHERE id=?", parent, in.Name, in.Kind, in.PageKey, in.URL, in.Icon, in.Sort, in.Visible, in.Permission, id)
		} else {
			_, e = tx.Exec("INSERT INTO menus(id,parent_id,name,kind,page_key,external_url,icon,sort,visible,permission) VALUES(?,?,?,?,?,?,?,?,?,?)", id, parent, in.Name, in.Kind, in.PageKey, in.URL, in.Icon, in.Sort, in.Visible, in.Permission)
		}
		if e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "保存菜单", idText(id))
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) deleteMenu(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
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
		var builtin bool
		var version, count int
		if e := tx.QueryRow("SELECT builtin,version FROM menus WHERE id=? AND deleted_at IS NULL", id).Scan(&builtin, &version); e == sql.ErrNoRows {
			return missing()
		} else if e != nil {
			return e
		}
		if version != in.Version {
			return conflict()
		}
		if builtin {
			return bad("内置菜单不能删除，可以隐藏")
		}
		if e := tx.QueryRow("SELECT count(*) FROM menus WHERE parent_id=? AND deleted_at IS NULL", id).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return bad("请先移动或删除子菜单")
		}
		if _, e := tx.Exec("UPDATE menus SET deleted_at=?,version=version+1 WHERE id=?", now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "删除菜单", idText(id))
	})
	return true, e
}
func (s *Server) resetMenus(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Revision string `json:"revision"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	e := s.Store.write(r.Context(), func(tx *sql.Tx) error {
		ms, e := readMenus(tx)
		if e != nil {
			return e
		}
		if menuRevision(ms) != in.Revision {
			return conflict()
		}
		for _, g := range []struct {
			id         int
			name, icon string
			sort       int
		}{{1, "观测与运维", "Activity", 10}, {2, "文件与资源", "FolderOpen", 20}, {3, "系统管理", "Settings2", 30}} {
			if _, e = tx.Exec("UPDATE menus SET parent_id=NULL,name=?,icon=?,sort=?,visible=1,permission='',version=version+1 WHERE id=?", g.name, g.icon, g.sort, g.id); e != nil {
				return e
			}
		}
		order := []string{"overview", "host", "monitors", "alerts", "files", "roles", "menus", "audit", "settings"}
		for i, key := range order {
			var p PageDefinition
			for _, item := range pages {
				if item.Key == key {
					p = item
				}
			}
			parent := 1
			sortValue := (i + 1) * 10
			if i == 4 {
				parent = 2
				sortValue = 10
			} else if i > 4 {
				parent = 3
				sortValue = (i - 4) * 10
			}
			if _, e = tx.Exec("UPDATE menus SET parent_id=?,name=?,icon=?,sort=?,visible=1,permission='',version=version+1 WHERE builtin=1 AND page_key=?", parent, p.Name, p.Icon, sortValue, key); e != nil {
				return e
			}
		}
		var group int64
		if e = tx.QueryRow("SELECT id FROM menus WHERE builtin_key='application-tools'").Scan(&group); e != nil {
			return e
		}
		if _, e = tx.Exec("UPDATE menus SET parent_id=NULL,name='应用与工具',icon='Box',sort=25,visible=1,permission='',version=version+1 WHERE id=?", group); e != nil {
			return e
		}
		for i, p := range pages[:3] {
			if _, e = tx.Exec("UPDATE menus SET parent_id=?,name=?,icon=?,sort=?,visible=1,permission='',version=version+1 WHERE builtin=1 AND page_key=?", group, p.Name, p.Icon, (i+1)*10, p.Key); e != nil {
				return e
			}
		}
		return auditRequest(tx, r, u.Name, "恢复内置导航", "")
	})
	return true, e
}
