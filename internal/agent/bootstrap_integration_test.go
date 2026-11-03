package agent_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lorvanto/monitor-go/internal/agent"
	"github.com/lorvanto/monitor-go/internal/app"
)

func bootstrapFixture(t *testing.T) (*app.Server, string) {
	t.Helper()
	s, err := app.New(app.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Store.Close() })
	token := strings.Repeat("a", 64)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	_, err = s.Store.DB.Exec("INSERT INTO hosts(id,hostname,source,enrollment_hash,enrollment_expires) VALUES(2,'bootstrap-test','agent',?,?)", hash, time.Now().Add(15*time.Minute).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	return s, token
}

func TestBootstrapLostEnrollmentResponseAndExpiredCodeSQLite(t *testing.T) {
	s, token := bootstrapFixture(t)
	var lost atomic.Bool
	var outage atomic.Bool
	outage.Store(true)
	handler := s.Handler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/enroll") && lost.CompareAndSwap(false, true) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, r)
			if recorder.Code != 200 {
				t.Errorf("enrollment did not commit: %d", recorder.Code)
			}
			if _, err := s.Store.DB.Exec("UPDATE hosts SET enrollment_expires=? WHERE id=2", time.Now().Add(-time.Minute).UnixMilli()); err != nil {
				t.Error(err)
			}
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			connection.Close()
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reports") && outage.Load() {
			w.WriteHeader(503)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	enrollment := filepath.Join(dir, "enrollment")
	if err := os.WriteFile(enrollment, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	if err := agent.Bootstrap(context.Background(), config, server.URL, enrollment, ""); err == nil {
		t.Fatal("lost response and unavailable reports should fail")
	}
	pending, err := agent.LoadOrCreate(config, "", "", "")
	if err != nil || pending.EnrollmentToken != token || len(pending.Credential) != 64 {
		t.Fatal("pending credential was not preserved")
	}
	var registered string
	if err = s.Store.DB.QueryRow("SELECT token_hash FROM hosts WHERE id=2").Scan(&registered); err != nil {
		t.Fatal(err)
	}
	if registered != fmt.Sprintf("%x", sha256.Sum256([]byte(pending.Credential))) {
		t.Fatal("server and disk credential differ after lost response")
	}
	outage.Store(false)
	if err = agent.Bootstrap(context.Background(), config, server.URL, enrollment, ""); err != nil {
		t.Fatal(err)
	}
	finished, err := agent.LoadOrCreate(config, "", "", "")
	if err != nil || finished.EnrollmentToken != "" || finished.Credential != pending.Credential {
		t.Fatal("expired-code recovery rotated or lost the credential")
	}
	var samples int
	if err = s.Store.DB.QueryRow("SELECT count(*) FROM host_samples WHERE host_id=2").Scan(&samples); err != nil || samples != 1 {
		t.Fatalf("first report was not accepted: samples=%d err=%v", samples, err)
	}
}

func TestAgentCLIOnceRegistersAndReports(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and executes the real Agent CLI")
	}
	s, token := bootstrapFixture(t)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/agent")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	config := filepath.Join(dir, "config.json")
	enrollment := filepath.Join(dir, "enrollment")
	if err := os.WriteFile(enrollment, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-once", "-server", server.URL, "-config", config, "-enrollment-file", enrollment)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI bootstrap: %v: %s", err, output)
	}
	if strings.Contains(string(output), token) {
		t.Fatal("CLI logged enrollment token")
	}
	var seen int64
	if err = s.Store.DB.QueryRow("SELECT last_seen FROM hosts WHERE id=2").Scan(&seen); err != nil || seen == 0 {
		t.Fatalf("CLI did not report online: %v", err)
	}
	info, err := os.Stat(config)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("CLI config is not private")
	}
}
