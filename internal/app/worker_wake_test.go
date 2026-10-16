package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWorkerDeadlinesAndCommittedWakeups(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	if got := s.checkDelay(ctx, true); got != workerFallback {
		t.Fatal(got)
	}
	if got := s.notificationDelay(ctx, true); got != workerFallback {
		t.Fatal(got)
	}
	// Initial seed/setup hints are coalesced; a failed transaction adds none.
	for _, ch := range []chan struct{}{s.Store.checkWake, s.Store.notificationWake} {
		select {
		case <-ch:
		default:
		}
	}
	if e := s.Store.write(ctx, func(*sql.Tx) error { return errors.New("rollback") }); e == nil {
		t.Fatal("missing rollback")
	}
	for _, ch := range []chan struct{}{s.Store.checkWake, s.Store.notificationWake} {
		select {
		case <-ch:
			t.Fatal("wake before commit")
		default:
		}
	}
	mustSQL(t, s, "INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds,next_check) VALUES(1,'future','TCP','localhost:80','',15,3,?)", now()+500)
	if got := s.checkDelay(ctx, true); got <= 0 || got > 500*time.Millisecond {
		t.Fatal(got)
	}
	if _, e := s.enqueue(ctx, 1); e != nil {
		t.Fatal(e)
	}
	select {
	case <-s.Store.checkWake:
	default:
		t.Fatal("missing post-commit wake")
	}
	if got := s.checkDelay(ctx, true); got != 10*time.Millisecond {
		t.Fatal(got)
	}
	if got := s.checkDelay(ctx, false); got != workerFallback {
		t.Fatal(got)
	}
}

func TestSchedulerDrainsFullPoolWithoutWaitingForPollingTicks(t *testing.T) {
	s, ts, _ := fixture(t)
	t.Logf("HTTP address: %s", ts.URL)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	defer func() { cancel(); s.Wait() }()
	begun := time.Now()
	e := s.Store.write(ctx, func(tx *sql.Tx) error {
		_, e := tx.Exec(`WITH RECURSIVE n(i) AS(VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<12)
 INSERT INTO monitors(id,name,type,target,group_name,interval_seconds,timeout_seconds,failure_threshold)
 SELECT i,'worker-test','HTTP',?,'',60,3,1 FROM n`, upstream.URL)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		s.Store.DB.QueryRow("SELECT count(*) FROM alerts WHERE state='open'").Scan(&count)
		if count == 12 {
			t.Logf("12 scheduled probes and alerts: %s", time.Since(begun))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scheduler did not drain work promptly")
}

func TestNotificationDeadlinePreservesGraceAndWakesForManualTest(t *testing.T) {
	s, ts, c := fixture(t)
	t.Logf("HTTP address: %s", ts.URL)
	received := make(chan time.Time, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- time.Now(); w.WriteHeader(200) }))
	defer upstream.Close()
	channel := decodeValue[map[string]string](t, c.call("POST", "/notification-channels", channelInput{Name: "Local", Kind: "webhook", Enabled: true, Config: channelConfig{URL: upstream.URL}}, 200))
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	defer func() { cancel(); s.Wait() }()
	created := now()
	e := s.Store.write(ctx, func(tx *sql.Tx) error {
		return transitionAlert(tx, "wake:test", "monitor", 1, "test", "test", "failure", "critical", true)
	})
	if e != nil {
		t.Fatal(e)
	}
	select {
	case at := <-received:
		if at.UnixMilli() < created+4990 {
			t.Fatal("notification grace shortened")
		}
		t.Logf("alert notification after %s (includes 5s grace)", time.Since(time.UnixMilli(created)))
	case <-time.After(9 * time.Second):
		t.Fatal("missed notification deadline")
	}
	started := time.Now()
	c.call("POST", "/notification-channels/"+channel["id"]+"/tests", map[string]any{"version": 1}, 200)
	select {
	case <-received:
		t.Logf("manual notification wake: %s", time.Since(started))
	case <-time.After(3 * time.Second):
		t.Fatal("manual notification not awakened")
	}
}
