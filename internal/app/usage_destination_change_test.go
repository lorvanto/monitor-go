package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUsageDestinationChangeRequiresConfirmationBeforeNetwork(t *testing.T) {
	previousConsole, _, _, previousValues := fakeUsageConsole(t)
	previous, previousClient, _ := usageFixture(t, previousConsole.URL)
	previousStatus := enableUsage(t, previousClient)
	bound := decodeValue[usageStatus](t, previousClient.call("POST", "/usage/bind", map[string]string{"code": strings.Repeat("a", 64)}, 200))
	if bound.Account == nil {
		t.Fatal("test installation was not bound to the previous Console account")
	}
	stored, _ := previousValues.Load("credential")
	previousCredential := stored.(string)
	dir := previous.Store.Dir
	previous.Store.Close()

	var confirmed atomic.Bool
	var requests, registrations, reports atomic.Int64
	var currentCredential atomic.Value
	nextConsole := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if !confirmed.Load() {
			t.Error("new Console contacted before confirmation of its destination")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if bytes.Contains(raw, []byte(previousCredential)) || strings.Contains(r.Header.Get("Authorization"), previousCredential) || bytes.Contains(raw, []byte(previousStatus.InstanceID)) {
			t.Error("previous installation identity or credential sent to the new Console")
		}
		var in map[string]any
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		id, _ := in["instanceId"].(string)
		if len(id) != 32 || len(in) != 6 {
			t.Error("unexpected registration/report fields")
		}
		switch r.URL.Path {
		case "/api/v1/instances/register":
			registrations.Add(1)
			credential, _ := in["credential"].(string)
			if len(credential) != 64 || r.Header.Get("Authorization") != "" {
				t.Error("new registration did not use a separate credential")
			}
			currentCredential.Store(credential)
		case "/api/v1/instances/report":
			reports.Add(1)
			credential, _ := currentCredential.Load().(string)
			if credential == "" || r.Header.Get("Authorization") != "Bearer "+credential {
				t.Error("report did not authenticate with the new registration")
			}
		default:
			t.Error("destination change attempted an account binding or another unexpected operation")
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(usageResult{InstanceID: id})
	}))
	defer nextConsole.Close()
	next, err := New(Config{DataDir: dir, ConsoleURL: nextConsole.URL, RequireActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(next.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); next.usage.loop(ctx) }()
	defer func() { local.Close(); cancel(); <-done; next.Store.Close() }()
	c := client(t, local.URL)
	c.login("admin", "Example-password-123!")
	status := decodeValue[usageStatus](t, c.call("GET", "/usage", nil, 200))
	if status.Enabled || !status.ConfirmationRequired || status.ConsoleURL != nextConsole.URL || status.InstanceID != "" || status.Account != nil {
		t.Fatal("destination change reused the previous confirmation, identity or account binding")
	}
	for _, stale := range []struct{ field, value string }{
		{"usageConsoleUrl", previousConsole.URL}, {"usageNoticeVersion", "1"},
	} {
		in := usageConfirmation(t, c)
		in[stale.field] = stale.value
		c.call("PATCH", "/usage", in, 422)
	}
	// Let the real worker run before asserting that neither startup nor rejected
	// confirmation sends anything, including a registration with the old key.
	time.Sleep(100 * time.Millisecond)
	if requests.Load() != 0 {
		t.Fatal("unconfirmed destination received network requests")
	}
	confirmed.Store(true)
	status = enableUsage(t, c)
	if !status.Enabled || status.ConfirmationRequired || status.InstanceID == previousStatus.InstanceID || len(status.InstanceID) != 32 || status.Account != nil {
		t.Fatal("new destination was not activated as a separate unbound installation")
	}
	waitUsage(t, func() bool { return reports.Load() == 1 })
	if registrations.Load() != 1 {
		t.Fatal("new destination did not receive exactly one confirmed registration")
	}
	status = decodeValue[usageStatus](t, c.call("GET", "/usage", nil, 200))
	if status.Account != nil {
		t.Fatal("previous Console account was automatically bound at the new destination")
	}
}
