package agent

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"github.com/lorvanto/monitor-go/internal/hoststats"
)

func TestRunEnrollmentPersistenceAndReport(t *testing.T) {
	type received struct {
		Path, Auth, UA string
		Body           json.RawMessage
	}
	messages := make(chan received, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		messages <- received{r.URL.Path, r.Header.Get("Authorization"), r.UserAgent(), b}
		w.WriteHeader(200)
	}))
	defer server.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	enrollment := filepath.Join(dir, "enrollment")
	os.WriteFile(enrollment, []byte(strings.Repeat("a", 64)), 0600)
	waits := []time.Duration{}
	collects := 0
	err := run(context.Background(), path, server.URL, enrollment, "", runDependencies{
		wait: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			if len(waits) > 2 {
				return context.Canceled
			}
			return nil
		},
		collect: func(context.Context) (hoststats.Report, error) {
			collects++
			return hoststats.Report{Protocol: 1, Hostname: "test-host", AgentVersion: buildinfo.Version}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, second := <-messages, <-messages
	var request map[string]string
	if err = json.Unmarshal(first.Body, &request); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadOrCreate(path, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EnrollmentToken != "" || len(cfg.Credential) != 64 || request["credential"] != cfg.Credential || request["token"] != strings.Repeat("a", 64) {
		t.Fatal("credentials not persisted correctly")
	}
	if first.Path != "/api/v1/agent/enroll" || first.Auth != "" || second.Path != "/api/v1/agent/reports" || second.Auth != "Bearer "+cfg.Credential || second.UA != buildinfo.UserAgent() {
		t.Fatalf("request metadata: %+v %+v", first, second)
	}
	var report hoststats.Report
	json.Unmarshal(second.Body, &report)
	if report.Hostname != "test-host" || len(report.ReportID) != 64 || report.Inventory.Source != "远程 Agent" || collects != 1 {
		t.Fatalf("report: %+v", report)
	}
	if !reflect.DeepEqual(waits, []time.Duration{0, 0, 15 * time.Second}) {
		t.Fatalf("waits: %v", waits)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("config mode")
	}
	reloaded, err := LoadOrCreate(path, "", "", "")
	if err != nil || reloaded.Credential != cfg.Credential {
		t.Fatal("credential rotated on reload")
	}
}

func TestRunRetryBackoffCapResetAndUnauthorizedExit(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		switch n {
		case 14:
			w.WriteHeader(200)
		case 16:
			w.WriteHeader(401)
		default:
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := save(path, Config{Server: server.URL, Credential: strings.Repeat("b", 64)}); err != nil {
		t.Fatal(err)
	}
	waits := []time.Duration{}
	err := run(context.Background(), path, "", "", "", runDependencies{
		wait: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			if len(waits) > 17 {
				return errors.New("loop did not stop")
			}
			return nil
		},
		collect: func(context.Context) (hoststats.Report, error) { return hoststats.Report{}, nil },
	})
	want := []time.Duration{0}
	for i := 1; i <= 13; i++ {
		want = append(want, time.Duration(min(i*5, 60))*time.Second)
	}
	want = append(want, 15*time.Second, 5*time.Second)
	if err == nil || count.Load() != 16 || !reflect.DeepEqual(waits, want) {
		t.Fatalf("count=%d waits=%v want=%v err=%v", count.Load(), waits, want, err)
	}
}

func TestClientRejectsRedirectAndSupportsPrivateCA(t *testing.T) {
	var leaked atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	cfg := Config{Server: redirect.URL, Credential: strings.Repeat("c", 64)}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if err = Send(context.Background(), client, cfg, "reports", map[string]string{}, true); err == nil || leaked.Load() != 0 {
		t.Fatal("redirect followed")
	}
	secure := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	secure.Config.ErrorLog = log.New(io.Discard, "", 0)
	secure.StartTLS()
	defer secure.Close()
	cfg.Server = secure.URL
	if err = Send(context.Background(), client, cfg, "reports", nil, true); err == nil {
		t.Fatal("untrusted CA accepted")
	}
	cfg.CAFile = filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(cfg.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: secure.Certificate().Raw}), 0600)
	trusted, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.CloseIdleConnections()
	if err = Send(context.Background(), trusted, cfg, "reports", nil, true); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(cfg.CAFile, []byte("not a certificate"), 0600)
	if _, err = NewClient(cfg); err == nil {
		t.Fatal("invalid CA accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = waitForReport(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
