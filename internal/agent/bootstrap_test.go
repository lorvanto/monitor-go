package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/lorvanto/monitor-go/internal/hoststats"
)

func TestBootstrapRetriesReportLimitAndPreservesCredential(t *testing.T) {
	var reports atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/reports" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("b", 64) {
			t.Error("unexpected authenticated report")
		}
		if reports.Add(1) == 1 {
			w.WriteHeader(429)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := save(path, Config{Server: server.URL, Credential: strings.Repeat("b", 64)}); err != nil {
		t.Fatal(err)
	}
	waits := 0
	err := bootstrap(context.Background(), path, server.URL, "", "", runDependencies{
		collect: func(context.Context) (hoststats.Report, error) { return hoststats.Report{}, nil },
		wait: func(_ context.Context, delay time.Duration) error {
			waits++
			if delay != 5*time.Second {
				t.Errorf("delay = %v", delay)
			}
			return nil
		},
	})
	if err != nil || reports.Load() != 2 || waits != 1 {
		t.Fatalf("reports=%d waits=%d err=%v", reports.Load(), waits, err)
	}
	cfg, err := LoadOrCreate(path, "", "", "")
	if err != nil || cfg.Credential != strings.Repeat("b", 64) {
		t.Fatal("bootstrap changed persistent credential")
	}
}

func TestBootstrapRejectsOtherServerAndRunningAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := save(path, Config{Server: "https://monitor.example.com", Credential: strings.Repeat("c", 64)}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := Bootstrap(context.Background(), path, "https://other.example.com", "", ""); err == nil {
		t.Fatal("accepted a different server for existing credentials")
	}
	lock := flock.New(path + ".lock")
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("lock: %v", err)
	}
	defer lock.Unlock()
	if err := Bootstrap(context.Background(), path, "", "", ""); err == nil {
		t.Fatal("bootstrap ignored a running Agent")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("rejected bootstrap changed config")
	}
}

func TestBootstrapReportFailureKeepsEnrollmentResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/reports") {
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	enrollment := filepath.Join(filepath.Dir(path), "enrollment")
	if err := os.WriteFile(enrollment, []byte(strings.Repeat("a", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	err := bootstrap(context.Background(), path, server.URL, enrollment, "", runDependencies{
		collect: func(context.Context) (hoststats.Report, error) { return hoststats.Report{}, nil },
	})
	var status *HTTPError
	if !errors.As(err, &status) || status.Status != 503 {
		t.Fatalf("first report should fail: %v", err)
	}
	cfg, err := LoadOrCreate(path, "", "", "")
	if err != nil || cfg.EnrollmentToken != "" || len(cfg.Credential) != 64 {
		t.Fatal("successful enrollment was not persisted after report failure")
	}
}
