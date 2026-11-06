package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lorvanto/monitor-go/internal/buildinfo"
)

const (
	usageUnavailable    = "Usage service is unavailable"
	usageRejected       = "Usage request was rejected"
	usageChanged        = "Usage reporting settings changed"
	usageRequired       = "Usage reporting is required and cannot be disabled"
	usageBindingRemoved = "Account binding is no longer supported"
)

var usageVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-]{0,99}$`)

type usagePayload struct {
	Version    string   `json:"version"`
	OS         string   `json:"os"`
	InstanceID string   `json:"instanceId"`
	ActiveDays []string `json:"activeDays"`
}
type usageStatus struct {
	Enabled              bool         `json:"enabled"`
	Required             bool         `json:"required"`
	NoticeVersion        string       `json:"noticeVersion"`
	ConfirmationRequired bool         `json:"confirmationRequired"`
	Configured           bool         `json:"configured"`
	ConsoleURL           string       `json:"consoleUrl"`
	InstanceID           string       `json:"instanceId"`
	LastReportAt         int64        `json:"lastReportAt"`
	NextReportAt         int64        `json:"nextReportAt"`
	LastError            string       `json:"lastError"`
	PendingActiveDays    []string     `json:"pendingActiveDays"`
	Preview              usagePayload `json:"preview"`
}
type usageState struct {
	Enabled                            bool
	ConsoleURL, InstanceID, Credential string
	Registered                         bool
	LastReportAt, NextReportAt         int64
	LastError                          string
	Failures                           int
}
type usageRemoteError struct{ status int }

func (e *usageRemoteError) Error() string { return usageRejected }
func (e *usageRemoteError) Unwrap() error { return &apiError{502, usageRejected} }

type usageResult struct {
	InstanceID string `json:"instanceId"`
	FirstSeen  int64  `json:"firstSeen"`
	LastSeen   int64  `json:"lastSeen"`
}

// Only this subsystem's goroutine performs scheduled network work. Its locks are
// never used by monitoring, notification delivery or the SQLite writer pool.
type usageReporter struct {
	server      *Server
	client      *http.Client
	origin      string
	wake        chan struct{}
	enabled     atomic.Bool
	changeMu    sync.Mutex
	mu          sync.Mutex
	session     context.Context
	cancel      context.CancelFunc
	stopped     bool
	activityDay string
	outboundMu  sync.Mutex
	inFlight    sync.WaitGroup
}

func normalizeConsoleURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return "", fmt.Errorf("console-url must be an HTTPS origin without credentials, path, query or fragment")
	}
	if u.Scheme != "https" {
		ip, e := netip.ParseAddr(u.Hostname())
		if u.Scheme != "http" || e != nil || !ip.IsLoopback() {
			return "", fmt.Errorf("console-url requires HTTPS; HTTP is allowed only for numeric loopback addresses")
		}
	}
	u.Path = ""
	return u.String(), nil
}
func newUsageReporter(s *Server) (*usageReporter, error) {
	origin, err := normalizeConsoleURL(s.cfg.ConsoleURL)
	if err != nil {
		return nil, err
	}
	deployment := s.cfg.Deployment
	if deployment == "" {
		deployment = "native"
	}
	if deployment != "native" && deployment != "docker" {
		return nil, errors.New("deployment must be native or docker")
	}
	p := &usageReporter{server: s, origin: origin, wake: make(chan struct{}, 1)}
	p.client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 4 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 2, MaxIdleConnsPerHost: 1, MaxConnsPerHost: 1,
	}}
	st, err := readUsageState(context.Background(), s.Store.DB)
	if err != nil {
		return nil, err
	}
	// Consent and credentials belong to one recipient. Changing the configured
	// origin always revokes local consent and never sends an old secret elsewhere.
	if st.ConsoleURL != origin {
		err = p.write(context.Background(), func(tx *sql.Tx) error {
			if _, e := tx.Exec(`UPDATE usage_settings SET enabled=0,console_url=?,instance_id='',credential='',registered=0,last_report_at=0,next_report_at=0,last_error='',failures=0,notice_version='',consent_at=0,activated_at=0 WHERE id=1`, origin); e != nil {
				return e
			}
			_, e := tx.Exec("DELETE FROM usage_activity")
			return e
		})
		if err != nil {
			return nil, err
		}
		st.Enabled = false
	}
	p.session, p.cancel = context.WithCancel(context.Background())
	p.enabled.Store(st.Enabled && origin != "")
	if !p.enabled.Load() {
		p.cancel()
	}
	return p, nil
}
func readUsageState(ctx context.Context, q rowQueryContext) (usageState, error) {
	var st usageState
	err := q.QueryRowContext(ctx, `SELECT enabled,console_url,instance_id,credential,registered,last_report_at,next_report_at,last_error,failures FROM usage_settings WHERE id=1`).Scan(&st.Enabled, &st.ConsoleURL, &st.InstanceID, &st.Credential, &st.Registered, &st.LastReportAt, &st.NextReportAt, &st.LastError, &st.Failures)
	return st, err
}

type rowQueryContext interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (p *usageReporter) write(ctx context.Context, fn func(*sql.Tx) error) error {
	// Usage writes deliberately do not wake the monitoring scheduler.
	tx, err := p.server.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (p *usageReporter) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
func (p *usageReporter) payload(id string, days []string) usagePayload {
	osName := runtime.GOOS
	switch osName {
	case "windows", "linux", "darwin", "freebsd", "openbsd", "netbsd":
	default:
		osName = "other"
	}
	return usagePayload{InstanceID: id, Version: usageVersion(buildinfo.Version), OS: osName, ActiveDays: days}
}

// Build descriptions are bounded technical identifiers, never arbitrary text.
func usageVersion(version string) string {
	if usageVersionPattern.MatchString(version) {
		return version
	}
	return "custom"
}
func (p *usageReporter) days(ctx context.Context) ([]string, error) {
	days := []string{}
	cutoff := time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	today := time.Now().UTC().Format("2006-01-02")
	rows, err := p.server.Store.reader().QueryContext(ctx, "SELECT day FROM usage_activity WHERE day>=? AND day<=? ORDER BY day LIMIT 7", cutoff, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var day string
		if err = rows.Scan(&day); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}
func (p *usageReporter) status(ctx context.Context) (usageStatus, error) {
	st, err := readUsageState(ctx, p.server.Store.reader())
	if err != nil {
		return usageStatus{}, err
	}
	days, err := p.days(ctx)
	if err != nil {
		return usageStatus{}, err
	}
	enabled := p.enabled.Load()
	status := usageStatus{Enabled: enabled, Required: true, NoticeVersion: activationNoticeVersion, ConfirmationRequired: !enabled, Configured: p.origin != "", ConsoleURL: p.origin, InstanceID: st.InstanceID, LastReportAt: st.LastReportAt, NextReportAt: st.NextReportAt, LastError: st.LastError, PendingActiveDays: days, Preview: p.payload(st.InstanceID, days)}
	return status, nil
}
func (s *Server) getUsage(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	return s.usage.status(r.Context())
}
func (s *Server) saveUsage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Enabled            *bool  `json:"enabled"`
		UsageConsent       bool   `json:"usageConsent"`
		UsageNoticeVersion string `json:"usageNoticeVersion"`
		UsageConsoleURL    string `json:"usageConsoleUrl"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Enabled == nil {
		return nil, bad("请求字段不正确")
	}
	if !*in.Enabled {
		return nil, &apiError{403, usageRequired}
	}
	p := s.usage
	p.changeMu.Lock()
	defer p.changeMu.Unlock()
	// Upgrades must preserve existing enabled installations and their historical
	// notice/identity. This idempotent request never invents a v2 confirmation.
	if p.enabled.Load() {
		return p.status(r.Context())
	}
	if err := s.validateActivationConsent(in.UsageConsent, in.UsageNoticeVersion, in.UsageConsoleURL); err != nil {
		return nil, err
	}
	activation, err := s.activateInstallation(r.Context())
	if err != nil {
		return nil, err
	}
	err = p.write(r.Context(), func(tx *sql.Tx) error {
		if err := activation.commit(r.Context(), tx); err != nil {
			return err
		}
		return auditRequest(tx, r, u.Name, "Usage reporting enabled", "usage")
	})
	if err != nil {
		return nil, err
	}
	p.resume()
	return p.status(r.Context())
}
func (p *usageReporter) resume() {
	p.mu.Lock()
	if !p.stopped && !p.enabled.Load() {
		p.session, p.cancel = context.WithCancel(context.Background())
		p.enabled.Store(true)
	}
	p.mu.Unlock()
	p.signal()
}
func (s *Server) usageActivity(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	var in struct{}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	p := s.usage
	if !p.enabled.Load() {
		return map[string]bool{"ok": true}, nil
	}
	today := time.Now().UTC().Format("2006-01-02")
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled.Load() || p.activityDay == today {
		return map[string]bool{"ok": true}, nil
	}
	err := p.write(r.Context(), func(tx *sql.Tx) error {
		if _, e := tx.Exec("DELETE FROM usage_activity WHERE day<? OR day>?", time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02"), today); e != nil {
			return e
		}
		_, e := tx.Exec("INSERT OR IGNORE INTO usage_activity(day) VALUES(?)", today)
		return e
	})
	if err == nil {
		p.activityDay = today
	}
	return map[string]bool{"ok": err == nil}, err
}

// Retired routes keep the existing permission and CSRF guards but never
// generate, consume or send an account-linking credential.
func (s *Server) retiredUsageBinding(_ http.ResponseWriter, _ *http.Request, _ *User) (any, error) {
	return nil, &apiError{http.StatusGone, usageBindingRemoved}
}

// begin serializes only outbound telemetry calls. The generation context makes
// shutdown cancel both network requests and their subsequent local writes.
func (p *usageReporter) begin(parent context.Context) (context.Context, func(), error) {
	p.outboundMu.Lock()
	p.mu.Lock()
	if !p.enabled.Load() || p.stopped {
		p.mu.Unlock()
		p.outboundMu.Unlock()
		return nil, nil, bad(usageChanged)
	}
	ctx, cancel := context.WithCancel(p.session)
	stop := context.AfterFunc(parent, cancel)
	p.inFlight.Add(1)
	p.mu.Unlock()
	if parent.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel(); p.inFlight.Done(); p.outboundMu.Unlock() }, nil
}
func (p *usageReporter) post(ctx context.Context, path, credential string, body any) (usageResult, error) {
	var out usageResult
	raw, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.origin+"/api/v2/instances/"+path, bytes.NewReader(raw))
	if err != nil {
		return out, bad(usageUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Monitor-Go")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	res, err := p.client.Do(req)
	if err != nil {
		return out, &apiError{502, usageUnavailable}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return out, &usageRemoteError{status: res.StatusCode}
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 16385))
	if err = decoder.Decode(&out); err != nil {
		return out, &apiError{502, usageUnavailable}
	}
	if decoder.Decode(new(any)) != io.EOF {
		return out, &apiError{502, usageUnavailable}
	}
	return out, nil
}

// report sends the permitted product version, OS family and anonymous
// installation/activity fields. User accounts, architecture and deployment are excluded.
func (p *usageReporter) report(parent context.Context) error {
	ctx, done, err := p.begin(parent)
	if err != nil {
		return err
	}
	defer done()
	st, err := readUsageState(ctx, p.server.Store.reader())
	if err != nil {
		return err
	}
	credential, err := p.server.Store.decrypt(st.Credential)
	if err != nil {
		return &apiError{502, usageUnavailable}
	}
	if !st.Registered {
		if err = p.register(ctx, st, credential); err != nil {
			return p.failed(ctx, st.Failures, err)
		}
	}
	days, err := p.days(ctx)
	if err != nil {
		return err
	}
	payload := p.payload(st.InstanceID, days)
	result, err := p.post(ctx, "report", credential, payload)
	var remote *usageRemoteError
	if errors.As(err, &remote) && remote.status == http.StatusNotFound {
		// Recreate an expired registration with its durable identity. Wrong
		// credentials (401/403) are never replaced or retried as registration.
		err = p.write(ctx, func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, "UPDATE usage_settings SET registered=0 WHERE id=1 AND enabled=1")
			return e
		})
		if err == nil {
			err = p.register(ctx, st, credential)
		}
		if err == nil {
			result, err = p.post(ctx, "report", credential, payload)
		}
	}
	if err != nil {
		return p.failed(ctx, st.Failures, err)
	}
	if result.InstanceID != st.InstanceID {
		return p.failed(ctx, st.Failures, &apiError{502, usageUnavailable})
	}
	return p.write(ctx, func(tx *sql.Tx) error {
		when := time.Now().Unix()
		if _, e := tx.ExecContext(ctx, `UPDATE usage_settings SET last_report_at=?,next_report_at=?,last_error='',failures=0 WHERE id=1 AND enabled=1`, when, when+usageReportDelay()); e != nil {
			return e
		}
		for _, day := range days {
			if _, e := tx.ExecContext(ctx, "DELETE FROM usage_activity WHERE day=?", day); e != nil {
				return e
			}
		}
		_, e := tx.ExecContext(ctx, "DELETE FROM usage_activity WHERE day<?", time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02"))
		return e
	})
}
func (p *usageReporter) registration(instanceID, credential string) any {
	payload := p.payload(instanceID, nil)
	return struct {
		InstanceID string `json:"instanceId"`
		Credential string `json:"credential"`
		Version    string `json:"version"`
		OS         string `json:"os"`
	}{instanceID, credential, payload.Version, payload.OS}
}
func (p *usageReporter) register(ctx context.Context, st usageState, credential string) error {
	result, err := p.post(ctx, "register", "", p.registration(st.InstanceID, credential))
	if err != nil {
		return err
	}
	if result.InstanceID != st.InstanceID {
		return &apiError{502, usageUnavailable}
	}
	return p.write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "UPDATE usage_settings SET registered=1 WHERE id=1 AND enabled=1")
		return e
	})
}

func usageReportDelay() int64 {
	// +/- 10% avoids synchronized traffic without delaying any monitoring work.
	n, err := rand.Int(rand.Reader, big.NewInt(4321))
	if err != nil {
		return 21600
	}
	return 19440 + n.Int64()
}
func usageRetryDelay(failures int) int64 {
	if failures < 1 {
		failures = 1
	}
	if failures > 10 {
		failures = 10
	}
	delay := int64(60) << (failures - 1)
	if delay > 21600 {
		delay = 21600
	}
	return delay
}
func (p *usageReporter) failed(ctx context.Context, failures int, cause error) error {
	if ctx.Err() != nil {
		return &apiError{409, usageChanged}
	}
	key := usageUnavailable
	var api *apiError
	if errors.As(cause, &api) && api.Message == usageRejected {
		key = usageRejected
	}
	if failures < 30 {
		failures++
	}
	err := p.write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "UPDATE usage_settings SET next_report_at=?,last_error=?,failures=? WHERE id=1 AND enabled=1", time.Now().Unix()+usageRetryDelay(failures), key, failures)
		return e
	})
	if err != nil {
		return err
	}
	return cause
}
func (p *usageReporter) loop(ctx context.Context) {
	defer func() {
		p.mu.Lock()
		p.stopped = true
		p.cancel()
		p.mu.Unlock()
		p.inFlight.Wait()
		p.client.CloseIdleConnections()
	}()
	retryNotBefore := time.Time{}
	for {
		wait := time.Duration(-1)
		if p.enabled.Load() {
			st, err := readUsageState(ctx, p.server.Store.reader())
			if err != nil {
				wait = time.Minute
			} else {
				wait = time.Until(time.Unix(st.NextReportAt, 0))
				if floor := time.Until(retryNotBefore); floor > wait {
					wait = floor
				}
				if wait < 0 {
					wait = 0
				}
			}
		}
		var timer *time.Timer
		var timerC <-chan time.Time
		if wait >= 0 {
			timer = time.NewTimer(wait)
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-p.wake:
			retryNotBefore = time.Time{}
			if timer != nil {
				timer.Stop()
			}
		case <-timerC:
			// All failures remain inside usage reporting; monitoring and alerts keep their
			// own queues and deadlines. Error details and credentials are never logged.
			if err := p.report(ctx); err != nil {
				retryNotBefore = time.Now().Add(time.Minute)
			} else {
				retryNotBefore = time.Time{}
			}
		}
	}
}
