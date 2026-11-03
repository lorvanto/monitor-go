package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/agentinstall"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Agent packages are immutable for a running server: the advertised checksum and
// download always describe the same bytes, even if a deployment replaces a file.
type agentPackage struct {
	body    []byte
	sha256  string
	modTime time.Time
}

func loadAgentPackages(directory string) map[string]agentPackage {
	packages := make(map[string]agentPackage)
	if directory == "" {
		return packages
	}
	for _, arch := range []string{"amd64", "arm64"} {
		name := filepath.Join(directory, "agent-linux-"+arch)
		info, err := os.Lstat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 || info.Size() == 0 {
			slog.Warn("Agent package is unavailable", "architecture", arch)
			continue
		}
		file, err := os.Open(name)
		if err != nil {
			slog.Warn("Agent package is unavailable", "architecture", arch)
			continue
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(info, opened) {
			file.Close()
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(file, (32<<20)+1))
		finished, finalStatErr := file.Stat()
		file.Close()
		if readErr != nil || finalStatErr != nil || !info.ModTime().Equal(finished.ModTime()) || info.Size() != finished.Size() || int64(len(body)) != info.Size() || len(body) == 0 || len(body) > 32<<20 {
			slog.Warn("Agent package is unavailable", "architecture", arch)
			continue
		}
		hash := sha256.Sum256(body)
		packages[arch] = agentPackage{body: body, sha256: hex.EncodeToString(hash[:]), modTime: info.ModTime()}
	}
	return packages
}

func (s *Server) agentDownloads(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	type platform struct {
		OS     string `json:"os"`
		Arch   string `json:"arch"`
		SHA256 string `json:"sha256"`
	}
	platforms := make([]platform, 0, len(s.agentPackages))
	for _, arch := range []string{"amd64", "arm64"} {
		if pkg, ok := s.agentPackages[arch]; ok {
			platforms = append(platforms, platform{OS: "linux", Arch: arch, SHA256: pkg.sha256})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	return map[string]any{"version": buildinfo.Version, "platforms": platforms}, nil
}

func (s *Server) agentInstallScript(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	body := agentinstall.Script()
	http.ServeContent(w, r, "install-agent.sh", time.Time{}, bytes.NewReader(body))
	return streamed{}, nil
}

func (s *Server) agentDownload(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	pkg, ok := s.agentPackages[r.PathValue("arch")]
	if r.PathValue("os") != "linux" || !ok {
		return nil, missing()
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="monitor-go-agent-linux-%s"`, r.PathValue("arch")))
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("ETag", `"`+pkg.sha256+`"`)
	w.Header().Set("X-Checksum-SHA256", pkg.sha256)
	// A package transfer can exceed the ordinary API response deadline on a
	// slower link. Keep the extended transfer bounded and do not change other APIs.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Minute))
	http.ServeContent(w, r, "agent-linux-"+r.PathValue("arch"), pkg.modTime, bytes.NewReader(pkg.body))
	return streamed{}, nil
}

func (s *Server) agentDownloadChecksum(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	pkg, ok := s.agentPackages[r.PathValue("arch")]
	if r.PathValue("os") != "linux" || !ok {
		return nil, missing()
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, pkg.sha256)
	return streamed{}, nil
}
