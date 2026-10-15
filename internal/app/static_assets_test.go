package app

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticAssetRepresentations(t *testing.T) {
	body := strings.Repeat("const greeting = 'Monitor Go';\n", 200)
	assets := fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte("<html>Monitor Go</html>")},
		"assets/app-aB12_C-9.js": &fstest.MapFile{Data: []byte(body)},
		"assets/plain.js":        &fstest.MapFile{Data: []byte("plain")},
		"favicon.svg":            &fstest.MapFile{Data: []byte("<svg/>")},
		"font.woff2":             &fstest.MapFile{Data: []byte("font")},
	}
	s, err := New(Config{DataDir: t.TempDir(), Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	handler := s.Handler()
	call := func(method, path string, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	url := "/assets/app-aB12_C-9.js"
	plain := call("GET", url, nil)
	zip := call("GET", url, map[string]string{"Accept-Encoding": "gzip"})
	if plain.Code != 200 || zip.Code != 200 {
		t.Fatalf("status %d %d", plain.Code, zip.Code)
	}
	if plain.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatal("missing hashed cache policy")
	}
	if plain.Header().Get("ETag") == zip.Header().Get("ETag") || strings.HasPrefix(zip.Header().Get("ETag"), "W/") {
		t.Fatal("representation validators must be distinct and strong")
	}
	if zip.Header().Get("Vary") != "Accept-Encoding" || zip.Header().Get("Content-Encoding") != "gzip" || zip.Header().Get("Content-Type") != plain.Header().Get("Content-Type") {
		t.Fatal("incorrect compressed headers")
	}
	reader, err := gzip.NewReader(bytes.NewReader(zip.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(decoded) != body {
		t.Fatal("compressed body corrupted")
	}
	for _, p := range []string{"/", "/index.html", "/favicon.svg", "/assets/plain.js"} {
		w := call("GET", p, nil)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" || w.Header().Get("ETag") == "" {
			t.Fatalf("unhashed asset cache %s: %v", p, w.Header())
		}
	}
	for _, tag := range []string{zip.Header().Get("ETag"), "W/" + zip.Header().Get("ETag"), `"other", ` + zip.Header().Get("ETag"), "*"} {
		w := call("GET", url, map[string]string{"Accept-Encoding": "gzip", "If-None-Match": tag})
		if w.Code != 304 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "" || w.Header().Get("ETag") != zip.Header().Get("ETag") || w.Header().Get("Vary") == "" {
			t.Fatalf("conditional response: %d %v %q", w.Code, w.Header(), w.Body.String())
		}
	}
	w := call("GET", url, map[string]string{"Accept-Encoding": "gzip", "If-None-Match": plain.Header().Get("ETag")})
	if w.Code != 200 {
		t.Fatal("matched a different representation")
	}
	head := call("HEAD", url, map[string]string{"Accept-Encoding": "gzip"})
	if head.Body.Len() != 0 || head.Header().Get("Content-Length") != strconv.Itoa(zip.Body.Len()) {
		t.Fatal("invalid HEAD length/body")
	}
	partial := call("GET", url, map[string]string{"Range": "bytes=0-4"})
	if partial.Code != 206 || partial.Body.String() != body[:5] {
		t.Fatal("range behavior regressed")
	}
	if call("POST", url, nil).Code != http.StatusMethodNotAllowed || call("GET", "/missing.js", nil).Code != 404 {
		t.Fatal("method or missing file handling regressed")
	}
	for _, w := range []*httptest.ResponseRecorder{plain, zip, head} {
		if w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("security headers lost")
		}
	}
	if call("GET", "/font.woff2", map[string]string{"Accept-Encoding": "gzip;q=1, identity;q=0"}).Code != 406 {
		t.Fatal("identity exclusion ignored")
	}
	// Startup representations remain stable even if a caller later mutates a test FS.
	assets["assets/app-aB12_C-9.js"].Data = []byte("changed")
	if call("GET", url, nil).Body.String() != body {
		t.Fatal("assets were not snapshotted")
	}
}

func TestStaticEncodingNegotiation(t *testing.T) {
	for _, tc := range []struct {
		header     string
		compressed bool
		coding     string
		ok         bool
	}{
		{"", true, "", true}, {"gzip", true, "gzip", true}, {"GZip;Q=1", true, "gzip", true},
		{"gzip;q=0", true, "", true}, {"gzip;q=0, *;q=1", true, "", true}, {"*;q=1", true, "gzip", true},
		{"br", true, "", true}, {"br, gzip;q=.5", true, "gzip", true}, {"gzip;q=0.5, identity;q=1", true, "", true},
		{"identity;q=0, gzip;q=0", true, "", false}, {"*;q=0", true, "", false}, {"gzip;q=1,*;q=0", true, "gzip", true},
		{"gzip;q=1,*;q=0", false, "", false}, {"gzip;q=NaN", true, "", true}, {"gzip;q=invalid", true, "", true},
		{"gzip;q=2", true, "", true}, {"*;q=0,identity;q=1", true, "", true},
	} {
		got, ok := staticEncoding(tc.header, tc.compressed)
		if got != tc.coding || ok != tc.ok {
			t.Errorf("%q: %q,%v want %q,%v", tc.header, got, ok, tc.coding, tc.ok)
		}
	}
}
