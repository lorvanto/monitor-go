package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/lorvanto/monitor-go/internal/agent"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
)

func TestSharedBuildVersionAndUserAgents(t *testing.T) {
	_, _, c := fixture(t)
	status := decodeValue[map[string]any](t, c.call("GET", "/status", nil, 200))
	system := decodeValue[map[string]any](t, c.call("GET", "/system", nil, 200))
	if status["version"] != buildinfo.Version || system["version"] != buildinfo.Version {
		t.Fatal("API versions diverge")
	}
	var calls atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") != buildinfo.UserAgent() {
			t.Errorf("%s User-Agent=%q", r.URL.Path, r.Header.Get("User-Agent"))
		}
		io.Copy(io.Discard, r.Body)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer receiver.Close()
	result := probe(context.Background(), Monitor{Type: "HTTP", Target: receiver.URL, Expected: 200, Timeout: 3}, "")
	if result.State != "healthy" {
		t.Fatalf("probe: %+v", result)
	}
	if _, err := sendWebhook(context.Background(), channelConfig{URL: receiver.URL}, notificationPayload{Event: "test"}); err != nil {
		t.Fatal(err)
	}
	client, err := agent.NewClient(agent.Config{Server: receiver.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if err = agent.Send(context.Background(), client, agent.Config{Server: receiver.URL}, "reports", map[string]any{}, false); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
}
