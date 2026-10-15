package app

import (
	"context"
	"database/sql"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Release struct {
	ID                 string `json:"id"`
	ApplicationID      string `json:"applicationId"`
	ApplicationName    string `json:"applicationName"`
	ApplicationEnabled bool   `json:"applicationEnabled"`
	Name               string `json:"name"`
	VersionName        string `json:"versionName"`
	Platform           string `json:"platform"`
	Arch               string `json:"arch"`
	Source             string `json:"source"`
	FileID             string `json:"fileId"`
	FileName           string `json:"fileName"`
	Size               int64  `json:"size"`
	SHA256             string `json:"sha256"`
	DownloadURL        string `json:"downloadUrl"`
	Notes              string `json:"notes"`
	State              string `json:"state"`
	EverPublished      bool   `json:"everPublished"`
	DownloadCount      int64  `json:"downloadCount"`
	Version            int    `json:"version"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
	PublishedAt        int64  `json:"publishedAt"`
}

const releaseColumns = "p.id,p.application_id,a.name,a.enabled,p.name,p.version_name,p.platform,p.arch,p.source,coalesce(CAST(p.file_id AS TEXT),''),coalesce(f.name,''),coalesce(f.size,0),coalesce(f.sha256,''),p.download_url,p.notes,p.state,p.ever_published,p.download_count,p.version,p.created_at,p.updated_at,coalesce(p.published_at,0)"
const releaseFrom = " FROM app_releases p JOIN applications a ON a.id=p.application_id LEFT JOIN files f ON f.id=p.file_id WHERE p.deleted_at IS NULL AND a.deleted_at IS NULL"

func scanRelease(row scanner) (Release, error) {
	var v Release
	e := row.Scan(&v.ID, &v.ApplicationID, &v.ApplicationName, &v.ApplicationEnabled, &v.Name, &v.VersionName, &v.Platform, &v.Arch, &v.Source, &v.FileID, &v.FileName, &v.Size, &v.SHA256, &v.DownloadURL, &v.Notes, &v.State, &v.EverPublished, &v.DownloadCount, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.PublishedAt)
	if e == sql.ErrNoRows {
		e = missing()
	}
	return v, e
}
func readRelease(q rowQuery, id any) (Release, error) {
	return scanRelease(q.QueryRow("SELECT "+releaseColumns+releaseFrom+" AND p.id=?", id))
}
func (s *Server) releaseList(r *http.Request, app string, machine bool, exposeURL bool) (any, error) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		return nil, bad("页码超过限制")
	}
	search := strings.TrimSpace(q.Get("q"))
	state := q.Get("state")
	if len(search) > 120 {
		return nil, bad("搜索词过长")
	}
	if state != "" && !slices.Contains([]string{"draft", "published", "withdrawn"}, state) {
		return nil, bad("未知发布状态")
	}
	if machine {
		state = "published"
	}
	filter := " AND (?='' OR p.application_id=?) AND (?='' OR p.state=?) AND (?='' OR instr(p.name||' '||p.version_name||' '||a.name,?)>0)"
	args := []any{app, app, state, state, search, search}
	if machine {
		filter += " AND a.enabled=1"
	}
	var total int
	if e := s.Store.DB.QueryRowContext(r.Context(), "SELECT count(*)"+releaseFrom+filter, args...).Scan(&total); e != nil {
		return nil, e
	}
	args = append(args, (page-1)*30)
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT "+releaseColumns+releaseFrom+filter+" ORDER BY p.id DESC LIMIT 30 OFFSET ?", args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		v, e := scanRelease(rows)
		if e != nil {
			return nil, e
		}
		if !exposeURL {
			v.DownloadURL = ""
		}
		out = append(out, v)
	}
	return map[string]any{"items": out, "total": total, "page": page, "pageSize": 30}, rows.Err()
}
func (s *Server) listReleases(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return s.releaseList(r, r.URL.Query().Get("applicationId"), false, u.Can("release:manage") || u.Can("release:download"))
}
func (s *Server) machineReleases(w http.ResponseWriter, r *http.Request, key machineIdentity) (any, error) {
	return s.releaseList(r, key.ApplicationID, true, false)
}

type releaseInput struct {
	ApplicationID string `json:"applicationId"`
	Name          string `json:"name"`
	VersionName   string `json:"versionName"`
	Platform      string `json:"platform"`
	Arch          string `json:"arch"`
	Source        string `json:"source"`
	FileID        string `json:"fileId"`
	DownloadURL   string `json:"downloadUrl"`
	Notes         string `json:"notes"`
	State         string `json:"state"`
	Version       int    `json:"version"`
}

var releaseVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+\-]{0,63}$`)

func (s *Server) saveRelease(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in releaseInput
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Notes = strings.TrimSpace(in.Notes)
	in.DownloadURL = strings.TrimSpace(in.DownloadURL)
	if in.Name == "" || len(in.Name) > 160 || !releaseVersion.MatchString(in.VersionName) || len(in.Notes) > 12000 || !slices.Contains([]string{"windows", "macos", "linux", "android", "ios", "web", "other"}, in.Platform) || !slices.Contains([]string{"amd64", "arm64", "universal", "other"}, in.Arch) || !slices.Contains([]string{"draft", "published", "withdrawn"}, in.State) {
		return nil, bad("请检查名称、版本号、平台、架构与发布状态")
	}
	if in.Source == "url" {
		link, e := url.Parse(in.DownloadURL)
		if e != nil || link.Scheme != "https" || link.Hostname() == "" || link.User != nil || len(in.DownloadURL) > 2048 || strings.ContainsAny(in.DownloadURL, "\r\n") || in.FileID != "" {
			return nil, bad("外部下载地址必须是 HTTPS 地址，不能包含用户名或密码")
		}
	} else if in.Source != "file" || in.FileID == "" || in.DownloadURL != "" {
		return nil, bad("请选择本地文件或外部下载地址")
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
		a, e := readApplication(tx, in.ApplicationID)
		if e != nil {
			return e
		}
		if in.State == "published" && !a.Enabled {
			return bad("应用停用时不能发布安装包")
		}
		var old Release
		if id > 0 {
			old, e = readRelease(tx, id)
			if e != nil {
				return e
			}
			if old.Version != in.Version {
				return conflict()
			}
			if old.EverPublished && (old.ApplicationID != in.ApplicationID || old.VersionName != in.VersionName || old.Platform != in.Platform || old.Arch != in.Arch || old.Source != in.Source || old.FileID != in.FileID || old.DownloadURL != in.DownloadURL) {
				return bad("已发布版本的应用、版本号、平台和安装文件不可替换，请创建新版本")
			}
			if old.EverPublished && in.State == "draft" {
				return bad("已发布的安装包只能下架，不能退回草稿")
			}
		}
		if in.Source == "file" {
			if (id == 0 || old.FileID != in.FileID || old.Source != "file") && !u.Can("file:download") {
				return &apiError{403, "关联本地文件还需要文件下载权限"}
			}
			f, e := readFile(tx, in.FileID)
			if e != nil {
				return e
			}
			if f.Kind != "file" || f.DeletedAt != 0 || !validObjectKey(f.ObjectKey) {
				return bad("安装文件不存在或已在回收站")
			}
		}
		var count int
		if e = tx.QueryRow("SELECT count(*) FROM app_releases WHERE application_id=? AND version_name=? AND platform=? AND arch=? AND id<>?", in.ApplicationID, in.VersionName, in.Platform, in.Arch, id).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return bad("该应用、版本号、平台与架构已存在（包含已归档版本）")
		}
		published := old.EverPublished || in.State == "published"
		var at any
		if old.PublishedAt > 0 {
			at = old.PublishedAt
		} else if in.State == "published" {
			at = now()
		}
		var fileID any
		if in.Source == "file" {
			fileID = in.FileID
		}
		args := []any{in.ApplicationID, in.Name, in.VersionName, in.Platform, in.Arch, in.Source, fileID, in.DownloadURL, in.Notes, in.State, published, at, now()}
		if id == 0 {
			args = append(args, u.ID, now())
			res, e := tx.Exec("INSERT INTO app_releases(application_id,name,version_name,platform,arch,source,file_id,download_url,notes,state,ever_published,published_at,updated_at,created_by,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", args...)
			if e != nil {
				return e
			}
			id, _ = res.LastInsertId()
		} else {
			args = append(args, id)
			if _, e = tx.Exec("UPDATE app_releases SET application_id=?,name=?,version_name=?,platform=?,arch=?,source=?,file_id=?,download_url=?,notes=?,state=?,ever_published=?,published_at=?,updated_at=?,version=version+1 WHERE id=?", args...); e != nil {
				return e
			}
		}
		return auditRequest(tx, r, u.Name, "保存安装包（"+map[string]string{"draft": "草稿", "published": "已发布", "withdrawn": "已下架"}[in.State]+"）", a.Name+" / "+in.VersionName+" / "+in.Name)
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) archiveRelease(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
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
		v, e := readRelease(tx, id)
		if e != nil {
			return e
		}
		if v.Version != in.Version {
			return conflict()
		}
		if _, e = tx.Exec("UPDATE app_releases SET deleted_at=?,state='withdrawn',version=version+1,updated_at=? WHERE id=?", now(), now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "归档安装包", v.ApplicationName+" / "+v.VersionName+" / "+v.Name)
	})
	return true, e
}

// Count successful transfers, including Range requests, rather than unique installations.
type transferWriter struct {
	http.ResponseWriter
	status int
	failed bool
}

func (w *transferWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *transferWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *transferWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	n, e := w.ResponseWriter.Write(p)
	if e != nil || n != len(p) {
		w.failed = true
	}
	return n, e
}
func (s *Server) downloadRelease(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	return s.serveRelease(w, r, "")
}
func (s *Server) machineDownload(w http.ResponseWriter, r *http.Request, key machineIdentity) (any, error) {
	return s.serveRelease(w, r, key.ApplicationID)
}
func (s *Server) serveRelease(w http.ResponseWriter, r *http.Request, app string) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	v, e := readRelease(s.Store.DB, id)
	if e != nil {
		return nil, e
	}
	if v.State != "published" || !v.ApplicationEnabled || (app != "" && v.ApplicationID != app) {
		return nil, missing()
	}
	if v.Source == "url" {
		// Never forward the caller's Bearer header to another host via an automatic redirect.
		if app != "" {
			return map[string]any{"source": "url", "url": v.DownloadURL, "message": "请对该地址发起独立请求，不要携带应用密钥"}, nil
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, v.DownloadURL, 302)
		if r.Method == "GET" {
			s.countDownload(v.ID)
		}
		return streamed{}, nil
	}
	f, e := readFile(s.Store.DB, v.FileID)
	if e != nil {
		return nil, e
	}
	if f.DeletedAt != 0 || f.Kind != "file" || !validObjectKey(f.ObjectKey) {
		return nil, missing()
	}
	body, e := s.Store.Files.Open("objects/" + f.ObjectKey)
	if e != nil {
		return nil, e
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Checksum-SHA256", f.SHA256)
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))
	tracked := &transferWriter{ResponseWriter: w}
	http.ServeContent(tracked, r, f.Name, time.UnixMilli(v.UpdatedAt), body)
	if r.Method == "GET" && !tracked.failed && (tracked.status == 200 || tracked.status == 206) {
		s.countDownload(v.ID)
	}
	return streamed{}, nil
}
func (s *Server) countDownload(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, e := s.Store.DB.ExecContext(ctx, "UPDATE app_releases SET download_count=download_count+1 WHERE id=?", id); e != nil {
		slog.Error("安装包下载计数失败", "release_id", id, "error", e)
	}
}
