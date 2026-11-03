package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentDistributionUsesMatchingImmutablePackages(t *testing.T) {
	directory := t.TempDir()
	binary := []byte("Monitor Go Linux amd64 package\n")
	path := filepath.Join(directory, "agent-linux-amd64")
	if err := os.WriteFile(path, binary, 0755); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{DataDir: t.TempDir(), AgentDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	get := func(path string) (*http.Response, []byte) {
		t.Helper()
		res, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res, body
	}
	res, manifest := get("/api/v1/agent/downloads")
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("manifest response: %d %s", res.StatusCode, manifest)
	}
	var decoded struct {
		Data struct {
			Platforms []struct{ OS, Arch, SHA256 string } `json:"platforms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(binary)
	digest := hex.EncodeToString(hash[:])
	if len(decoded.Data.Platforms) != 1 || decoded.Data.Platforms[0].OS != "linux" || decoded.Data.Platforms[0].Arch != "amd64" || decoded.Data.Platforms[0].SHA256 != digest {
		t.Fatalf("incorrect platform manifest: %s", manifest)
	}
	// Publishing a replacement requires restarting the server. Downloads and
	// checksums seen by an ongoing installation cannot diverge during an upgrade.
	if err := os.WriteFile(path, []byte("replacement package"), 0755); err != nil {
		t.Fatal(err)
	}
	res, body := get("/api/v1/agent/download/linux/amd64")
	if res.StatusCode != 200 || string(body) != string(binary) || res.Header.Get("X-Checksum-SHA256") != digest || res.Header.Get("ETag") != `"`+digest+`"` {
		t.Fatalf("binary response: %d %q", res.StatusCode, body)
	}
	res, checksum := get("/api/v1/agent/download/linux/amd64/sha256")
	if res.StatusCode != 200 || string(checksum) != digest+"\n" {
		t.Fatalf("checksum response: %d %q", res.StatusCode, checksum)
	}
	request, _ := http.NewRequest("GET", ts.URL+"/api/v1/agent/download/linux/amd64", nil)
	request.Header.Set("Range", "bytes=0-6")
	ranged, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer ranged.Body.Close()
	rangeBody, _ := io.ReadAll(ranged.Body)
	if ranged.StatusCode != 206 || string(rangeBody) != string(binary[:7]) {
		t.Fatalf("range response: %d %q", ranged.StatusCode, rangeBody)
	}
	for _, path := range []string{"/api/v1/agent/download/windows/amd64", "/api/v1/agent/download/linux/arm64", "/api/v1/agent/download/linux/ppc64/sha256"} {
		res, body := get(path)
		if res.StatusCode != 404 {
			t.Fatalf("unsupported package %s returned %d: %s", path, res.StatusCode, body)
		}
	}
	res, script := get("/api/v1/agent/install.sh")
	if res.StatusCode != 200 || !strings.HasPrefix(string(script), "#!/bin/sh") || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/x-shellscript") || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("installer response: %d %q", res.StatusCode, script)
	}
	if _, err := os.Stat(filepath.Join(s.Store.Dir, "monitor.db")); err != nil {
		t.Fatal("distribution test must use an actual SQLite database:", err)
	}
}

func TestAgentDistributionOmitsUnsafeAndUnavailableFiles(t *testing.T) {
	directory := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("not an agent"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(directory, "agent-linux-amd64")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "agent-linux-arm64"), 0700); err != nil {
		t.Fatal(err)
	}
	if packages := loadAgentPackages(directory); len(packages) != 0 {
		t.Fatal("non-regular files must not be distributed")
	}
	if packages := loadAgentPackages(""); len(packages) != 0 {
		t.Fatal("unset package directory should provide no downloads")
	}
	if packages := loadAgentPackages(filepath.Join(directory, "missing")); len(packages) != 0 {
		t.Fatal("unbuilt native installations should provide no downloads")
	}
}
