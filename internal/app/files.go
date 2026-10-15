package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type FileEntry struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	MIME      string `json:"mime"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Version   int    `json:"version"`
	DeletedAt int64  `json:"deletedAt"`
	TrashRoot string `json:"trashRoot"`
	ObjectKey string `json:"-"`
	Creator   string `json:"creator"`
}

const fileColumns = "f.id,coalesce(CAST(f.parent_id AS TEXT),''),f.kind,f.name,f.mime,f.size,f.sha256,f.width,f.height,f.created_at,f.updated_at,f.version,coalesce(f.deleted_at,0),coalesce(CAST(f.trash_root AS TEXT),''),f.object_key,coalesce(u.display_name,'系统')"

type scanner interface{ Scan(...any) error }

func scanFile(row scanner) (FileEntry, error) {
	var f FileEntry
	e := row.Scan(&f.ID, &f.ParentID, &f.Kind, &f.Name, &f.MIME, &f.Size, &f.SHA256, &f.Width, &f.Height, &f.CreatedAt, &f.UpdatedAt, &f.Version, &f.DeletedAt, &f.TrashRoot, &f.ObjectKey, &f.Creator)
	return f, e
}
func readFile(q rowQuery, id any) (FileEntry, error) {
	f, e := scanFile(q.QueryRow("SELECT "+fileColumns+" FROM files f LEFT JOIN users u ON u.id=f.created_by WHERE f.id=? AND f.purged_at IS NULL", id))
	if e == sql.ErrNoRows {
		return f, missing()
	}
	return f, e
}
func fileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || len(name) > 200 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) {
		return "", bad("名称为 1–200 字节，不能包含路径分隔符或控制字符")
	}
	return name, nil
}
func fileDBError(e error) error {
	if e != nil && strings.Contains(e.Error(), "UNIQUE constraint failed: files.parent_id, files.name") {
		return bad("目录内已存在同名文件或文件夹，请换一个名称")
	}
	return e
}
func activeFolder(q rowQuery, id string) error {
	f, e := readFile(q, id)
	if e != nil {
		return e
	}
	if f.Kind != "folder" || f.DeletedAt != 0 {
		return bad("目标目录不存在或已在回收站")
	}
	return nil
}
func (s *Server) fileTree(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT "+fileColumns+" FROM files f LEFT JOIN users u ON u.id=f.created_by WHERE f.kind='folder' AND f.deleted_at IS NULL AND f.purged_at IS NULL ORDER BY f.id")
	if e != nil {
		return nil, e
	}
	items := []FileEntry{}
	for rows.Next() {
		v, e := scanFile(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	settings, e := readSettings(s.Store.DB)
	if e != nil {
		return nil, e
	}
	var used, trash, count int64
	e = s.Store.DB.QueryRowContext(r.Context(), "SELECT coalesce(sum(CASE WHEN object_key<>'' THEN size ELSE 0 END),0),coalesce(sum(CASE WHEN deleted_at IS NOT NULL AND purged_at IS NULL AND id=trash_root THEN 1 ELSE 0 END),0),coalesce(sum(CASE WHEN kind='file' AND deleted_at IS NULL AND purged_at IS NULL THEN 1 ELSE 0 END),0) FROM files").Scan(&used, &trash, &count)
	return map[string]any{"items": items, "uploadLimitMB": settings.UploadLimitMB, "storageLimitMB": settings.StorageLimitMB, "defaultFolder": settings.DefaultFolder, "storageUsed": used, "trashCount": trash, "fileCount": count}, e
}
func (s *Server) listFiles(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	q := r.URL.Query()
	parent := q.Get("parent")
	if parent == "" {
		parent = "1"
	}
	trash := q.Get("trash") == "1"
	search := strings.TrimSpace(q.Get("q"))
	if len(search) > 120 {
		return nil, bad("搜索词过长")
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		return nil, bad("页码过大")
	}
	where := "f.purged_at IS NULL AND f.id<>1"
	args := []any{}
	if trash {
		where += " AND f.deleted_at IS NOT NULL AND f.id=f.trash_root"
	} else {
		if e := activeFolder(s.Store.DB, parent); e != nil {
			return nil, e
		}
		where += " AND f.deleted_at IS NULL AND f.parent_id=?"
		args = append(args, parent)
	}
	if search != "" {
		where += " AND instr(lower(f.name),lower(?))>0"
		args = append(args, search)
	}
	var count int
	if e := s.Store.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM files f WHERE "+where, args...).Scan(&count); e != nil {
		return nil, e
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT "+fileColumns+" FROM files f LEFT JOIN users u ON u.id=f.created_by WHERE "+where+" ORDER BY f.kind DESC,f.updated_at DESC,f.id DESC LIMIT 50 OFFSET ?", append(args, (page-1)*50)...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []FileEntry{}
	for rows.Next() {
		v, e := scanFile(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	return map[string]any{"items": items, "total": count, "page": page}, rows.Err()
}
func (s *Server) createFolder(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Name     string `json:"name"`
		ParentID string `json:"parentId"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	name, e := fileName(in.Name)
	if e != nil {
		return nil, e
	}
	if in.ParentID == "" {
		in.ParentID = "1"
	}
	var id int64
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		if e := activeFolder(tx, in.ParentID); e != nil {
			return e
		}
		var count int
		if e := tx.QueryRow("SELECT count(*) FROM files WHERE kind='folder' AND purged_at IS NULL").Scan(&count); e != nil {
			return e
		}
		if count >= 1000 {
			return bad("最多支持 1000 个文件夹（含回收站）")
		}
		if e := validateFileMove(tx, "", in.ParentID); e != nil {
			return e
		}
		res, e := tx.Exec("INSERT INTO files(parent_id,kind,name,created_by,created_at,updated_at) VALUES(?,'folder',?,?,?,?)", in.ParentID, name, u.ID, now(), now())
		if e != nil {
			return fileDBError(e)
		}
		id, _ = res.LastInsertId()
		return auditRequest(tx, r, u.Name, "创建文件夹", name)
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	select {
	case s.uploadSlots <- struct{}{}:
		defer func() { <-s.uploadSlots }()
	default:
		return nil, &apiError{429, "上传任务较多，请稍后重试"}
	}
	settings, e := readSettings(s.Store.DB)
	if e != nil {
		return nil, e
	}
	limit := settings.UploadLimitMB * 1048576
	http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Minute))
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(11 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, limit+65536)
	reader, e := r.MultipartReader()
	if e != nil {
		return nil, bad("请使用文件上传表单")
	}
	parent := r.URL.Query().Get("parent")
	if parent == "" {
		parent = settings.DefaultFolder
	}
	if e = activeFolder(s.Store.DB, parent); e != nil {
		return nil, e
	}
	part, e := reader.NextPart()
	if e != nil || part.FormName() != "file" || part.FileName() == "" {
		return nil, bad("每个请求必须包含一个 file 文件")
	}
	defer part.Close()
	name, e := fileName(part.FileName())
	if e != nil {
		return nil, e
	}
	key := randomToken()
	stage := "staging/" + key
	object := "objects/" + key
	f, e := s.Store.Files.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	defer s.Store.Files.Remove(stage)
	hash := sha256.New()
	size, e := io.Copy(io.MultiWriter(f, hash), io.LimitReader(part, limit+1))
	if e != nil {
		return nil, bad("上传中断或内容超过限制")
	}
	if size == 0 || size > limit {
		return nil, badf("文件不能为空，且不能超过 %d MB", settings.UploadLimitMB)
	}
	if _, e = reader.NextPart(); e != io.EOF {
		return nil, bad("每次只能上传一个文件")
	}
	if e = f.Sync(); e != nil {
		return nil, e
	}
	f.Seek(0, io.SeekStart)
	head := make([]byte, 512)
	n, _ := f.Read(head)
	mimeType := http.DetectContentType(head[:n])
	width, height := 0, 0
	f.Seek(0, io.SeekStart)
	if config, _, err := image.DecodeConfig(f); err == nil {
		width, height = config.Width, config.Height
	}
	if e = f.Close(); e != nil {
		return nil, e
	}
	var id int64
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		if e := activeFolder(tx, parent); e != nil {
			return e
		}
		cfg, e := readSettings(tx)
		if e != nil {
			return e
		}
		if size > cfg.UploadLimitMB*1048576 {
			return bad("上传上限已变更，请重新上传")
		}
		var used int64
		if e = tx.QueryRow("SELECT coalesce(sum(size),0) FROM files WHERE object_key<>''").Scan(&used); e != nil {
			return e
		}
		if used+size > cfg.StorageLimitMB*1048576 {
			return bad("存储空间不足，请清理回收站或调整配额")
		}
		res, e := tx.Exec("INSERT INTO files(parent_id,kind,name,object_key,mime,size,sha256,width,height,created_by,created_at,updated_at) VALUES(?,'file',?,?,?,?,?,?,?,?,?,?)", parent, name, key, mimeType, size, hex.EncodeToString(hash.Sum(nil)), width, height, u.ID, now(), now())
		if e != nil {
			return fileDBError(e)
		}
		id, _ = res.LastInsertId()
		if e = auditRequest(tx, r, u.Name, "上传文件", name); e != nil {
			return e
		}
		return s.Store.Files.Rename(stage, object)
	})
	if e != nil {
		s.Store.Files.Remove(object)
		return nil, e
	}
	return readFile(s.Store.DB, id)
}
func validateFileMove(tx *sql.Tx, id, parent string) error {
	depth := 0
	for parent != "" {
		if parent == id {
			return bad("不能移动到自己或子目录")
		}
		f, e := readFile(tx, parent)
		if e != nil {
			return e
		}
		depth++
		if depth > 16 {
			return bad("文件夹层级最多为 16 层")
		}
		parent = f.ParentID
	}
	if id != "" {
		var sub int
		if e := tx.QueryRow("WITH RECURSIVE sub(id,depth) AS(SELECT id,1 FROM files WHERE id=? UNION ALL SELECT f.id,s.depth+1 FROM files f JOIN sub s ON f.parent_id=s.id WHERE f.deleted_at IS NULL AND f.purged_at IS NULL) SELECT coalesce(max(depth),1) FROM sub", id).Scan(&sub); e != nil {
			return e
		}
		if depth+sub > 17 {
			return bad("移动后文件夹层级超过限制")
		}
	}
	return nil
}
func (s *Server) updateFile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	if id == 1 {
		return nil, bad("根目录不可修改")
	}
	var in struct {
		Name     string `json:"name"`
		ParentID string `json:"parentId"`
		Version  int    `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	name, e := fileName(in.Name)
	if e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		f, e := readFile(tx, id)
		if e != nil {
			return e
		}
		if f.Version != in.Version {
			return conflict()
		}
		if f.DeletedAt != 0 {
			return bad("请先从回收站恢复")
		}
		if e = activeFolder(tx, in.ParentID); e != nil {
			return e
		}
		if e = validateFileMove(tx, f.ID, in.ParentID); e != nil {
			return e
		}
		_, e = tx.Exec("UPDATE files SET name=?,parent_id=?,updated_at=?,version=version+1 WHERE id=?", name, in.ParentID, now(), id)
		if e != nil {
			return fileDBError(e)
		}
		return auditRequest(tx, r, u.Name, "更新文件或目录", f.Name+" → "+name)
	})
	return true, e
}

const activeSubtree = "WITH RECURSIVE sub(id) AS(SELECT id FROM files WHERE id=? UNION ALL SELECT f.id FROM files f JOIN sub s ON f.parent_id=s.id WHERE f.deleted_at IS NULL AND f.purged_at IS NULL) "

func (s *Server) trashFile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	if id == 1 {
		return nil, bad("根目录不可删除")
	}
	var in struct {
		Version int `json:"version"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		f, e := readFile(tx, id)
		if e != nil {
			return e
		}
		if f.Version != in.Version {
			return conflict()
		}
		if f.DeletedAt != 0 {
			return bad("文件已在回收站")
		}
		var n int
		if e = tx.QueryRow(activeSubtree+"SELECT count(*) FROM sub WHERE id=(SELECT default_folder FROM system_settings WHERE id=1)", id).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return bad("该目录包含默认上传目录，请先在系统设置中更换默认目录")
		}
		if e = tx.QueryRow(activeSubtree+"SELECT count(*) FROM app_releases WHERE deleted_at IS NULL AND file_id IN (SELECT id FROM sub)", id).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return bad("该文件或目录仍被安装包引用，请先归档对应安装包")
		}
		if _, e = tx.Exec(activeSubtree+"UPDATE files SET deleted_at=?,trash_root=?,version=version+1 WHERE id IN (SELECT id FROM sub)", id, now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "移入回收站", f.Name)
	})
	return true, e
}
func (s *Server) restoreFile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version  int    `json:"version"`
		ParentID string `json:"parentId"`
		Name     string `json:"name"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		f, e := readFile(tx, id)
		if e != nil {
			return e
		}
		if f.Version != in.Version {
			return conflict()
		}
		if f.DeletedAt == 0 || f.TrashRoot != f.ID {
			return bad("请从回收站的根条目恢复")
		}
		parent := in.ParentID
		if parent == "" {
			parent = f.ParentID
		}
		if e = activeFolder(tx, parent); e != nil {
			return bad("原目录已不存在，请选择一个恢复目录")
		}
		if e = validateFileMove(tx, f.ID, parent); e != nil {
			return e
		}
		// Include the deleted batch depth; active descendants alone do not describe a trashed folder.
		var depth int
		if e = tx.QueryRow("WITH RECURSIVE sub(id,depth) AS(SELECT id,1 FROM files WHERE id=? UNION ALL SELECT f.id,s.depth+1 FROM files f JOIN sub s ON f.parent_id=s.id WHERE f.trash_root=? AND f.purged_at IS NULL) SELECT max(depth) FROM sub", id, id).Scan(&depth); e != nil {
			return e
		}
		p := parent
		for p != "" {
			node, e := readFile(tx, p)
			if e != nil {
				return e
			}
			depth++
			p = node.ParentID
		}
		if depth > 17 {
			return bad("恢复后目录层级超过限制")
		}
		name := f.Name
		if in.Name != "" {
			name, e = fileName(in.Name)
			if e != nil {
				return e
			}
		}
		if _, e = tx.Exec("UPDATE files SET parent_id=?,name=? WHERE id=?", parent, name, id); e != nil {
			return fileDBError(e)
		}
		if _, e = tx.Exec("UPDATE files SET deleted_at=NULL,trash_root=NULL,updated_at=?,version=version+1 WHERE trash_root=? AND purged_at IS NULL", now(), id); e != nil {
			return fileDBError(e)
		}
		return auditRequest(tx, r, u.Name, "恢复文件或目录", name)
	})
	return true, e
}
func (s *Server) purgeFile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	id, e := pathID(r)
	if e != nil {
		return nil, e
	}
	var in struct {
		Version int    `json:"version"`
		Confirm string `json:"confirm"`
	}
	if e = decode(r, &in); e != nil {
		return nil, e
	}
	if in.Confirm != "DELETE" && in.Confirm != "永久删除" {
		return nil, bad("请输入“永久删除”确认")
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error {
		f, e := readFile(tx, id)
		if e != nil {
			return e
		}
		if f.Version != in.Version {
			return conflict()
		}
		if f.DeletedAt == 0 || f.TrashRoot != f.ID {
			return bad("只能永久删除回收站根条目")
		}
		if _, e = tx.Exec("UPDATE files SET purged_at=?,version=version+1 WHERE trash_root=? AND purged_at IS NULL", now(), id); e != nil {
			return e
		}
		return auditRequest(tx, r, u.Name, "永久删除文件或目录", f.Name)
	})
	if e != nil {
		return nil, e
	}
	s.collectPurgedFiles(r.Context())
	return true, nil
}

// Tombstones retain an object key until unlink succeeds, making interrupted deletion retryable.
func (s *Server) collectPurgedFiles(ctx context.Context) {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	s.imageMu.Lock()
	defer s.imageMu.Unlock()
	rows, e := s.Store.DB.QueryContext(ctx, "SELECT id,object_key FROM files WHERE purged_at IS NOT NULL AND object_key<>'' LIMIT 500")
	if e != nil {
		return
	}
	type item struct {
		id  int64
		key string
	}
	items := []item{}
	for rows.Next() {
		var v item
		if rows.Scan(&v.id, &v.key) == nil {
			items = append(items, v)
		}
	}
	rows.Close()
	for _, v := range items {
		if !validObjectKey(v.key) {
			continue
		}
		e = s.Store.Files.Remove("objects/" + v.key)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			continue
		}
		if cache, e := s.Store.Files.Open("cache"); e == nil {
			entries, _ := cache.ReadDir(-1)
			cache.Close()
			for _, f := range entries {
				if strings.HasPrefix(f.Name(), v.key+"-") {
					s.Store.Files.Remove("cache/" + f.Name())
				}
			}
		}
		s.Store.DB.ExecContext(ctx, "UPDATE files SET object_key='' WHERE id=? AND purged_at IS NOT NULL", v.id)
	}
}
func validObjectKey(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && strings.ToLower(s) == s
}
func (s *Server) openFileContent(r *http.Request) (FileEntry, *os.File, error) {
	id, e := pathID(r)
	if e != nil {
		return FileEntry{}, nil, e
	}
	f, e := readFile(s.Store.DB, id)
	if e != nil {
		return f, nil, e
	}
	if f.Kind != "file" || f.DeletedAt != 0 || !validObjectKey(f.ObjectKey) {
		return f, nil, missing()
	}
	body, e := s.Store.Files.Open("objects/" + f.ObjectKey)
	if errors.Is(e, os.ErrNotExist) {
		return f, nil, &apiError{410, "文件内容已丢失，请检查数据目录备份"}
	}
	return f, body, e
}
func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	f, body, e := s.openFileContent(r)
	if e != nil {
		return nil, e
	}
	defer body.Close()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(11 * time.Minute))
	http.ServeContent(w, r, f.Name, time.UnixMilli(f.UpdatedAt), body)
	return streamed{}, nil
}
func (s *Server) previewFile(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	f, body, e := s.openFileContent(r)
	if e != nil {
		return nil, e
	}
	defer body.Close()
	out := map[string]any{"file": f, "kind": "unsupported"}
	if previewableImage(f) {
		out["kind"] = "image"
	} else if strings.HasPrefix(f.MIME, "text/") || strings.Contains(f.MIME, "json") || strings.Contains(f.MIME, "xml") {
		b, e := io.ReadAll(io.LimitReader(body, 262145))
		if e != nil {
			return nil, e
		}
		truncated := len(b) > 262144
		if truncated {
			b = b[:262144]
			for trim := 0; trim < utf8.UTFMax-1 && len(b) > 0 && !utf8.Valid(b); trim++ {
				b = b[:len(b)-1]
			}
		}
		if utf8.Valid(b) {
			out["kind"] = "text"
			out["text"] = string(b)
			out["truncated"] = truncated
		}
	}
	return out, nil
}
func previewableImage(f FileEntry) bool {
	return f.Width > 0 && f.Height > 0 && int64(f.Width)*int64(f.Height) <= 24000000 && f.Width <= 16384 && f.Height <= 16384 && (f.MIME == "image/png" || f.MIME == "image/jpeg" || f.MIME == "image/gif" || f.MIME == "image/webp")
}

// Recover interrupted uploads without touching referenced originals or recent transfers.
func (s *Server) cleanupUploadResidue(ctx context.Context) {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for _, sub := range []string{"staging", "objects"} {
		dir, e := s.Store.Files.Open(sub)
		if e != nil {
			continue
		}
		for time.Now().Before(deadline) && ctx.Err() == nil {
			entries, e := dir.ReadDir(100)
			for _, entry := range entries {
				if !validObjectKey(entry.Name()) || entry.IsDir() {
					continue
				}
				info, err := entry.Info()
				if err != nil || time.Since(info.ModTime()) < 24*time.Hour {
					continue
				}
				if sub == "objects" {
					var refs int
					if err = s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE object_key=?", entry.Name()).Scan(&refs); err != nil || refs != 0 {
						continue
					}
				}
				s.Store.Files.Remove(sub + "/" + entry.Name())
			}
			if e != nil {
				break
			}
		}
		dir.Close()
	}
}
