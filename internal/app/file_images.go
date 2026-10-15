package app

import (
	"database/sql"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type ImageStyle struct {
	NameKey string `json:"nameKey,omitempty"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Fit     string `json:"fit"`
	Version int    `json:"version"`
}

func (s *Server) listImageStyles(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	rows, e := s.Store.DB.QueryContext(r.Context(), "SELECT id,name,width,height,fit,version FROM image_styles WHERE deleted_at IS NULL ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []ImageStyle{}
	for rows.Next() {
		var v ImageStyle
		if e = rows.Scan(&v.ID, &v.Name, &v.Width, &v.Height, &v.Fit, &v.Version); e != nil {
			return nil, e
		}
		if v.Version == 1 && (v.ID == "1" && v.Name == "列表缩略图" || v.ID == "2" && v.Name == "文档插图") {
			v.NameKey = v.Name
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (s *Server) saveImageStyle(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in ImageStyle
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || in.Width < 16 || in.Width > 2048 || in.Height < 16 || in.Height > 2048 || (in.Fit != "contain" && in.Fit != "cover") {
		return nil, bad("请输入名称、16–2048 像素尺寸以及有效缩放方式")
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
			res, e := tx.Exec("UPDATE image_styles SET name=?,width=?,height=?,fit=?,version=version+1 WHERE id=? AND version=? AND deleted_at IS NULL", in.Name, in.Width, in.Height, in.Fit, id, in.Version)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return conflict()
			}
		} else {
			var n int
			if e := tx.QueryRow("SELECT count(*) FROM image_styles WHERE deleted_at IS NULL").Scan(&n); e != nil {
				return e
			}
			if n >= 30 {
				return bad("最多配置 30 个图片样式")
			}
			res, e := tx.Exec("INSERT INTO image_styles(name,width,height,fit) VALUES(?,?,?,?)", in.Name, in.Width, in.Height, in.Fit)
			if e != nil {
				return e
			}
			id, _ = res.LastInsertId()
		}
		return auditRequest(tx, r, u.Name, "保存图片样式", in.Name)
	})
	return map[string]string{"id": idText(id)}, e
}
func (s *Server) deleteImageStyle(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
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
		res, e := tx.Exec("UPDATE image_styles SET deleted_at=?,version=version+1 WHERE id=? AND version=? AND deleted_at IS NULL", now(), id, in.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict()
		}
		return auditRequest(tx, r, u.Name, "归档图片样式", idText(id))
	})
	return true, e
}
func (s *Server) fileImage(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	// Serialize decoding to bound memory; the source and destination are both dimension-limited.
	s.imageMu.Lock()
	defer s.imageMu.Unlock()
	f, body, e := s.openFileContent(r)
	if e != nil {
		return nil, e
	}
	defer body.Close()
	if !previewableImage(f) {
		return nil, bad("仅预览不超过 2400 万像素的 PNG、JPEG、GIF 和 WebP 图片")
	}
	style := ImageStyle{ID: "preview", Width: 1600, Height: 1200, Fit: "contain", Version: 1}
	if r.URL.Query().Get("size") == "thumb" {
		style = ImageStyle{ID: "thumb", Width: 320, Height: 240, Fit: "cover", Version: 1}
	}
	if id := r.URL.Query().Get("style"); id != "" {
		if _, e := strconv.ParseInt(id, 10, 64); e != nil {
			return nil, bad("样式编号无效")
		}
		e = s.Store.DB.QueryRowContext(r.Context(), "SELECT id,name,width,height,fit,version FROM image_styles WHERE id=? AND deleted_at IS NULL", id).Scan(&style.ID, &style.Name, &style.Width, &style.Height, &style.Fit, &style.Version)
		if e == sql.ErrNoRows {
			return nil, missing()
		}
		if e != nil {
			return nil, e
		}
	}
	cacheKey := fmt.Sprintf("cache/%s-%s-%d.png", f.ObjectKey, style.ID, style.Version)
	cached, e := s.Store.Files.Open(cacheKey)
	if os.IsNotExist(e) {
		var src image.Image
		src, _, e = image.Decode(body)
		if e != nil {
			return nil, bad("图片无法解码")
		}
		bounds := src.Bounds()
		sw, sh := bounds.Dx(), bounds.Dy()
		dw, dh := style.Width, style.Height
		if style.Fit == "contain" {
			if dw > sw {
				dw = sw
			}
			if dh > sh {
				dh = sh
			}
			if int64(sw)*int64(dh) > int64(sh)*int64(dw) {
				dh = max(1, sh*dw/sw)
			} else {
				dw = max(1, sw*dh/sh)
			}
		} else {
			if int64(sw)*int64(dh) > int64(sh)*int64(dw) {
				width := max(1, sh*dw/dh)
				bounds.Min.X += (sw - width) / 2
				bounds.Max.X = bounds.Min.X + width
			} else {
				height := max(1, sw*dh/dw)
				bounds.Min.Y += (sh - height) / 2
				bounds.Max.Y = bounds.Min.Y + height
			}
		}
		dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)
		temp := cacheKey + ".tmp"
		out, e := s.Store.Files.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return nil, e
		}
		e = png.Encode(out, dst)
		closeErr := out.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			s.Store.Files.Remove(temp)
			return nil, e
		}
		if e = s.Store.Files.Rename(temp, cacheKey); e != nil {
			s.Store.Files.Remove(temp)
			return nil, e
		}
		cached, e = s.Store.Files.Open(cacheKey)
		s.pruneImageCache(cacheKey)
	}
	if e != nil {
		return nil, e
	}
	defer cached.Close()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("download") == "1" {
		if !u.Can("file:download") {
			return nil, &apiError{403, "当前账号没有下载权限"}
		}
		w.Header().Set("Content-Disposition", "attachment; filename=styled-image.png")
	}
	http.ServeContent(w, r, "preview.png", time.Time{}, cached)
	return streamed{}, nil
}

// Cache is disposable and bounded independently of the original-file quota.
func (s *Server) pruneImageCache(keep string) {
	dir, e := s.Store.Files.Open("cache")
	if e != nil {
		return
	}
	defer dir.Close()
	entries, _ := dir.ReadDir(-1)
	infos := []os.FileInfo{}
	var total int64
	for _, v := range entries {
		if info, e := v.Info(); e == nil && !info.IsDir() {
			infos = append(infos, info)
			total += info.Size()
		}
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].ModTime().Before(infos[j].ModTime()) })
	for _, v := range infos {
		if total <= 128*1048576 {
			break
		}
		if "cache/"+v.Name() == keep {
			continue
		}
		if s.Store.Files.Remove("cache/"+v.Name()) == nil {
			total -= v.Size()
		}
	}
}
func (s *Server) clearImageCache(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	s.imageMu.Lock()
	defer s.imageMu.Unlock()
	dir, e := s.Store.Files.Open("cache")
	if e != nil {
		return nil, e
	}
	entries, e := dir.ReadDir(-1)
	dir.Close()
	if e != nil {
		return nil, e
	}
	count := 0
	for _, v := range entries {
		if !v.IsDir() {
			if e = s.Store.Files.Remove("cache/" + v.Name()); e != nil {
				return nil, e
			}
			count++
		}
	}
	e = s.Store.write(r.Context(), func(tx *sql.Tx) error { return auditRequest(tx, r, u.Name, "清理图片缓存", fmt.Sprint(count)) })
	return map[string]int{"removed": count}, e
}
