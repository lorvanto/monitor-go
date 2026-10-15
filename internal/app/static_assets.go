package app

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type staticAsset struct {
	plain, compressed                         []byte
	etag, gzipETag, contentType, cacheControl string
}

var hashedAssetPath = regexp.MustCompile(`^/assets/.+-[A-Za-z0-9_-]{8}\.[A-Za-z0-9]+$`)

// Embedded build assets are immutable for the lifetime of a process. Generate
// validators and compressed representations once, before accepting requests.
func newStaticHandler(assets fs.FS) (http.Handler, error) {
	files := map[string]staticAsset{}
	err := fs.WalkDir(assets, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(assets, name)
		if err != nil {
			return err
		}
		item := staticAsset{plain: content, etag: fmt.Sprintf(`"%x"`, sha256.Sum256(content)), contentType: mime.TypeByExtension(path.Ext(name)), cacheControl: "no-cache"}
		if item.contentType == "" {
			item.contentType = http.DetectContentType(content)
		}
		if hashedAssetPath.MatchString("/" + name) {
			item.cacheControl = "public, max-age=31536000, immutable"
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".js", ".css", ".html", ".svg", ".json":
			var buffer bytes.Buffer
			writer := gzip.NewWriter(&buffer)
			if _, err = writer.Write(content); err != nil {
				return err
			}
			if err = writer.Close(); err != nil {
				return err
			}
			item.compressed = buffer.Bytes()
			item.gzipETag = fmt.Sprintf(`"%x"`, sha256.Sum256(item.compressed))
		}
		files["/"+name] = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	fallback := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		key := r.URL.Path
		if strings.HasSuffix(key, "/") {
			key += "index.html"
		}
		item, ok := files[key]
		if !ok {
			fallback.ServeHTTP(w, r)
			return
		}
		content, etag := item.plain, item.etag
		w.Header().Set("Vary", "Accept-Encoding")
		coding, acceptable := staticEncoding(strings.Join(r.Header.Values("Accept-Encoding"), ","), item.compressed != nil)
		if !acceptable {
			http.Error(w, "No acceptable content encoding", http.StatusNotAcceptable)
			return
		}
		if coding == "gzip" {
			content, etag = item.compressed, item.gzipETag
			w.Header().Set("Content-Encoding", "gzip")
		}
		w.Header().Set("Content-Type", item.contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.Header().Set("Cache-Control", item.cacheControl)
		w.Header().Set("ETag", etag)
		// ServeContent handles conditional requests, HEAD, ranges, weak comparison
		// for If-None-Match and strong comparison for If-Match/If-Range.
		http.ServeContent(w, r, path.Base(key), time.Time{}, bytes.NewReader(content))
	}), nil
}

func staticEncoding(header string, compressed bool) (string, bool) {
	if strings.TrimSpace(header) == "" {
		return "", true
	}
	qualities := map[string]float64{}
	for _, entry := range strings.Split(header, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ";")
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		quality := 1.0
		for _, parameter := range parts[1:] {
			pair := strings.SplitN(strings.TrimSpace(parameter), "=", 2)
			if len(pair) != 2 || !strings.EqualFold(strings.TrimSpace(pair[0]), "q") {
				quality = 0
				continue
			}
			value, err := strconv.ParseFloat(strings.TrimSpace(pair[1]), 64)
			if err != nil || !(value >= 0 && value <= 1) {
				quality = 0
			} else {
				quality = value
			}
		}
		qualities[name] = quality
	}
	gzipQ, exists := qualities["gzip"]
	if !exists {
		gzipQ = qualities["*"]
	}
	identityQ, explicitIdentity := qualities["identity"]
	if !explicitIdentity {
		identityQ = 1
		if star, ok := qualities["*"]; ok && star == 0 {
			identityQ = 0
		}
	}
	// Implicit identity is a fallback; an explicitly preferred identity wins.
	if compressed && gzipQ > 0 && (!explicitIdentity || gzipQ >= identityQ) {
		return "gzip", true
	}
	return "", identityQ > 0
}
