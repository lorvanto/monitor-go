package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func localSSLCertificate(t *testing.T) (string, *tls.Config, int64) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "local SSL test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(48 * time.Hour).Truncate(time.Second),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:        true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	up := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	up.Config.ErrorLog = log.New(io.Discard, "", 0)
	up.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	up.StartTLS()
	t.Cleanup(up.Close)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return up.Listener.Addr().String(), &tls.Config{RootCAs: roots}, cert.NotAfter.UnixMilli()
}

func manualMonitorJob(t *testing.T, s *Server, c *testClient, id string) (int64, Monitor) {
	t.Helper()
	queued := decodeValue[map[string]string](t, c.call("POST", "/monitors/"+id+"/checks", map[string]any{}, 202))
	job, err := strconv.ParseInt(queued["jobId"], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	m, err := scanMonitor(s.Store.DB.QueryRow("SELECT "+monitorColumns+" FROM monitors WHERE id=?", id))
	if err != nil {
		t.Fatal(err)
	}
	return job, m
}

func TestSSLProbeCertificateExpiryAndTrust(t *testing.T) {
	target, config, expiry := localSSLCertificate(t)
	m := Monitor{Type: "SSL", Target: target, Timeout: 2, SSLDays: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, days := range []int{1, 3} {
		m.SSLDays = days
		result := probeWithTLSConfig(ctx, m, "", config)
		want := "healthy"
		if days == 3 {
			want = "warning"
		}
		if result.State != want || result.CertificateExpiresAt != expiry {
			t.Fatalf("days=%d: %+v; want %s expiry=%d", days, result, want, expiry)
		}
	}
	if result := probe(ctx, m, ""); result.State != "failed" || result.CertificateExpiresAt != 0 {
		t.Fatalf("untrusted certificate was accepted: %+v", result)
	}
}

func TestSSLConfirmationSchedulingAndManualRecovery(t *testing.T) {
	s, _, c := fixture(t)
	target, config, expiry := localSSLCertificate(t)
	in := monitorPayload(target)
	in.Type, in.Interval, in.SSLDays = "SSL", 86400, 1
	id := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))["id"]
	// Old installations can have a different threshold and warnings accumulated
	// as failures. Neither value may bypass SSL's three connection failures.
	mustSQL(t, s, "UPDATE monitors SET failure_threshold=7 WHERE id=?", id)
	run := func(trusted bool) {
		t.Helper()
		job, m := manualMonitorJob(t, s, c, id)
		var err error
		if trusted {
			result := probeWithTLSConfig(context.Background(), m, "", config)
			err = s.completeCheck(context.Background(), job, m, result)
		} else {
			err = s.execute(context.Background(), job)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	assertResult := func(wantState string, wantFailures int, delay time.Duration, wantExpiry int64) {
		t.Helper()
		var state string
		var failures int
		var next, certificate, historyCertificate int64
		if err := s.Store.DB.QueryRow("SELECT state,failures,next_check,certificate_expires_at FROM monitors WHERE id=?", id).Scan(&state, &failures, &next, &certificate); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.DB.QueryRow("SELECT certificate_expires_at FROM check_results WHERE monitor_id=? ORDER BY id DESC LIMIT 1", id).Scan(&historyCertificate); err != nil {
			t.Fatal(err)
		}
		remaining := time.Until(time.UnixMilli(next))
		if state != wantState || failures != wantFailures || remaining < delay-3*time.Second || remaining > delay+time.Second || certificate != wantExpiry || historyCertificate != wantExpiry {
			t.Fatalf("state=%s failures=%d delay=%s expiry=%d/%d; want %s %d %s %d", state, failures, remaining, certificate, historyCertificate, wantState, wantFailures, delay, wantExpiry)
		}
	}
	assertActive := func(want int, severity string) {
		t.Helper()
		var count int
		if err := s.Store.DB.QueryRow("SELECT count(*) FROM alerts WHERE fingerprint=? AND state!='resolved' AND severity=?", "monitor:"+id, severity).Scan(&count); err != nil || count != want {
			t.Fatalf("active %s alerts=%d, want %d: %v", severity, count, want, err)
		}
	}
	run(true)
	assertResult("healthy", 0, 24*time.Hour, expiry)
	for failures := 1; failures <= 3; failures++ {
		run(false)
		delay := time.Minute
		if failures == 3 {
			delay = 24 * time.Hour
		}
		assertResult("failed", failures, delay, 0)
		want := 0
		if failures == 3 {
			want = 1
		}
		assertActive(want, "critical")
	}
	run(false)
	assertResult("failed", 4, 24*time.Hour, 0)
	run(true)
	assertResult("healthy", 0, 24*time.Hour, expiry)
	assertActive(0, "critical")
	mustSQL(t, s, "UPDATE monitors SET ssl_days=3 WHERE id=?", id)
	run(true)
	assertResult("warning", 0, 24*time.Hour, expiry)
	assertActive(1, "warning")
	run(false)
	assertResult("failed", 1, time.Minute, 0)
	run(true)
	assertResult("warning", 0, 24*time.Hour, expiry)
	assertActive(1, "warning")
	run(false)
	assertResult("failed", 1, time.Minute, 0)
	// A warning from a pre-upgrade version must start a fresh failure sequence.
	mustSQL(t, s, "UPDATE monitors SET state='warning',failures=99 WHERE id=?", id)
	run(false)
	assertResult("failed", 1, time.Minute, 0)
	assertActive(0, "critical")
	mustSQL(t, s, "UPDATE monitors SET ssl_days=1 WHERE id=?", id)
	run(true)
	assertResult("healthy", 0, 24*time.Hour, expiry)
	assertActive(0, "warning")
}

func TestSSLCompletionDiscardsPausedOrEditedResults(t *testing.T) {
	s, _, c := fixture(t)
	for _, change := range []string{"pause", "edit"} {
		t.Run(change, func(t *testing.T) {
			in := monitorPayload("127.0.0.1:443")
			in.Type, in.Interval = "SSL", 86400
			id := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))["id"]
			job, m := manualMonitorJob(t, s, c, id)
			mustSQL(t, s, "UPDATE jobs SET state='running' WHERE id=?", job)
			if change == "pause" {
				c.call("PATCH", "/monitors/"+id, map[string]any{"version": 1, "enabled": false}, 200)
			} else {
				in.Target = "127.0.0.1:8443"
				c.call("PUT", "/monitors/"+id, monitorEditInput{monitorInput: in, Version: 1}, 200)
			}
			if err := s.completeCheck(context.Background(), job, m, CheckResult{At: now(), State: "warning", CertificateExpiresAt: now() + 86400000}); err != nil {
				t.Fatal(err)
			}
			var results, alerts int
			var jobState string
			var next, expiry int64
			if err := s.Store.DB.QueryRow("SELECT (SELECT count(*) FROM check_results WHERE monitor_id=?),(SELECT count(*) FROM alerts WHERE resource_id=?),next_check,certificate_expires_at FROM monitors WHERE id=?", id, id, id).Scan(&results, &alerts, &next, &expiry); err != nil {
				t.Fatal(err)
			}
			if err := s.Store.DB.QueryRow("SELECT state FROM jobs WHERE id=?", job).Scan(&jobState); err != nil {
				t.Fatal(err)
			}
			if results != 0 || alerts != 0 || next != 0 || expiry != 0 || jobState != "cancelled" {
				t.Fatalf("stale result committed: results=%d alerts=%d next=%d expiry=%d job=%s", results, alerts, next, expiry, jobState)
			}
		})
	}
}

func TestOtherMonitorThresholdAndSchedulingUnchanged(t *testing.T) {
	s, _, c := fixture(t)
	for _, state := range []string{"failed", "warning"} {
		t.Run(state, func(t *testing.T) {
			in := monitorPayload("http://127.0.0.1/")
			in.Threshold = 3
			id := decodeValue[map[string]string](t, c.call("POST", "/monitors", in, 201))["id"]
			for i := 1; i <= 4; i++ {
				job, m := manualMonitorJob(t, s, c, id)
				anchor := now() + 900000
				mustSQL(t, s, "UPDATE monitors SET next_check=? WHERE id=?", anchor, id)
				resultState := state
				wantFailures, wantAlerts := i, 0
				if i == 3 {
					wantAlerts = 1
				} else if i == 4 {
					resultState, wantFailures = "healthy", 0
				}
				if err := s.completeCheck(context.Background(), job, m, CheckResult{At: now(), State: resultState, CertificateExpiresAt: 123}); err != nil {
					t.Fatal(err)
				}
				var failures, alerts int
				var next, expiry, historyExpiry int64
				if err := s.Store.DB.QueryRow("SELECT failures,next_check,certificate_expires_at,(SELECT certificate_expires_at FROM check_results WHERE monitor_id=? ORDER BY id DESC LIMIT 1),(SELECT count(*) FROM alerts WHERE fingerprint=? AND state!='resolved') FROM monitors WHERE id=?", id, "monitor:"+id, id).Scan(&failures, &next, &expiry, &historyExpiry, &alerts); err != nil {
					t.Fatal(err)
				}
				if failures != wantFailures || alerts != wantAlerts || next != anchor || expiry != 0 || historyExpiry != 0 {
					t.Fatalf("failures=%d alerts=%d next=%d expiry=%d/%d; want failures=%d alerts=%d next=%d", failures, alerts, next, expiry, historyExpiry, wantFailures, wantAlerts, anchor)
				}
			}
		})
	}
}
