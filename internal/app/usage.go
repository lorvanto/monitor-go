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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lorvanto/monitor-go/internal/buildinfo"
)

const (
	usageNotConfigured = "Usage reporting is not configured"
	usageNotEnabled    = "Enable usage reporting before binding an account"
	usageInvalidCode   = "Invalid binding code"
	usageUnavailable   = "Usage service is unavailable"
	usageRejected      = "Usage request was rejected"
	usageChanged       = "Usage reporting settings changed"
)

var usageCodePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var usageLoginPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
var usageVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-]{0,99}$`)

type usageAccount struct {
	Login string `json:"login"`
}
type usagePayload struct {
	InstanceID string   `json:"instanceId"`
	Version    string   `json:"version"`
	OS         string   `json:"os"`
	Arch       string   `json:"arch"`
	Deployment string   `json:"deployment"`
	ActiveDays []string `json:"activeDays"`
}
type usageStatus struct {
	Enabled           bool          `json:"enabled"`
	Configured        bool          `json:"configured"`
	ConsoleURL        string        `json:"consoleUrl"`
	InstanceID        string        `json:"instanceId"`
	LastReportAt      int64         `json:"lastReportAt"`
	NextReportAt      int64         `json:"nextReportAt"`
	LastError         string        `json:"lastError"`
	Account           *usageAccount `json:"account"`
	PendingActiveDays []string      `json:"pendingActiveDays"`
	Preview           usagePayload  `json:"preview"`
}
type usageState struct {
	Enabled                            bool
	ConsoleURL, InstanceID, Credential string
	Registered                         bool
	LastReportAt, NextReportAt         int64
	LastError, AccountLogin            string
	Failures                           int
}
type usageRemoteError struct{ status int }

func (e *usageRemoteError) Error() string { return usageRejected }
func (e *usageRemoteError) Unwrap() error { return &apiError{502, usageRejected} }

type usageResult struct {
	InstanceID string        `json:"instanceId"`
	FirstSeen  int64         `json:"firstSeen"`
	LastSeen   int64         `json:"lastSeen"`
	Account    *usageAccount `json:"account"`
}

// Only this subsystem's goroutine performs scheduled network work. Its locks are
// never used by monitoring, notification delivery or the SQLite writer pool.
type usageReporter struct {
	server      *Server
	client      *http.Client
	origin      string
	deployment  string
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
	p := &usageReporter{server: s, origin: origin, deployment: deployment, wake: make(chan struct{}, 1)}
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
			if _, e := tx.Exec(`UPDATE usage_settings SET enabled=0,console_url=?,instance_id='',credential='',registered=0,last_report_at=0,next_report_at=0,last_error='',account_login='',failures=0 WHERE id=1`, origin); e != nil {
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
	err := q.QueryRowContext(ctx, `SELECT enabled,console_url,instance_id,credential,registered,last_report_at,next_report_at,last_error,account_login,failures FROM usage_settings WHERE id=1`).Scan(&st.Enabled, &st.ConsoleURL, &st.InstanceID, &st.Credential, &st.Registered, &st.LastReportAt, &st.NextReportAt, &st.LastError, &st.AccountLogin, &st.Failures)
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
	osName, arch := runtime.GOOS, runtime.GOARCH
	switch osName {
	case "windows", "linux", "darwin", "freebsd", "openbsd", "netbsd":
	default:
		osName = "other"
	}
	switch arch {
	case "amd64", "arm64", "386", "arm":
	default:
		arch = "other"
	}
	version := usageVersion(buildinfo.Version)
	return usagePayload{InstanceID: id, Version: version, OS: osName, Arch: arch, Deployment: p.deployment, ActiveDays: days}
}
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
	status := usageStatus{Enabled: p.enabled.Load(), Configured: p.origin != "", ConsoleURL: p.origin, InstanceID: st.InstanceID, LastReportAt: st.LastReportAt, NextReportAt: st.NextReportAt, LastError: st.LastError, PendingActiveDays: days, Preview: p.payload(st.InstanceID, days)}
	if st.AccountLogin != "" {
		status.Account = &usageAccount{Login: st.AccountLogin}
	}
	return status, nil
}
func (s *Server) getUsage(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	return s.usage.status(r.Context())
}
func (s *Server) saveUsage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Enabled == nil {
		return nil, bad("请求字段不正确")
	}
	p := s.usage
	p.changeMu.Lock()
	defer p.changeMu.Unlock()
	if *in.Enabled && p.origin == "" {
		return nil, bad(usageNotConfigured)
	}
	if *in.Enabled == p.enabled.Load() {
		// A failed opt-out write leaves the worker paused but consent still enabled
		// on disk. A repeated request must retry persistence, not report success and
		// allow the next process to resume reporting from stale durable consent.
		st, err := readUsageState(r.Context(), p.server.Store.reader())
		if err != nil {
			return nil, err
		}
		if st.Enabled == *in.Enabled {
			return p.status(r.Context())
		}
	}
	if !*in.Enabled {
		// Prevent new requests before cancelling existing ones. Wait before replying,
		// so no request can be started after a successful opt-out response.
		p.mu.Lock()
		p.enabled.Store(false)
		p.cancel()
		p.activityDay = ""
		p.mu.Unlock()
		p.inFlight.Wait()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	err := p.write(ctx, func(tx *sql.Tx) error {
		st, e := readUsageState(ctx, tx)
		if e != nil {
			return e
		}
		if *in.Enabled && st.InstanceID == "" {
			st.InstanceID = randomToken()[:32]
			st.Credential, e = s.Store.encrypt(randomToken())
			if e != nil {
				return e
			}
		}
		next := int64(0)
		action := "Usage reporting disabled"
		if *in.Enabled {
			next = time.Now().Unix()
			action = "Usage reporting enabled"
		}
		if _, e = tx.ExecContext(ctx, `UPDATE usage_settings SET enabled=?,instance_id=?,credential=?,next_report_at=?,last_error='',failures=0 WHERE id=1`, *in.Enabled, st.InstanceID, st.Credential, next); e != nil {
			return e
		}
		if !*in.Enabled {
			if _, e = tx.ExecContext(ctx, "DELETE FROM usage_activity"); e != nil {
				return e
			}
		}
		return auditRequest(tx, r, u.Name, action, "usage")
	})
	if err != nil {
		return nil, err
	}
	if *in.Enabled {
		p.mu.Lock()
		if !p.stopped {
			p.session, p.cancel = context.WithCancel(context.Background())
			p.enabled.Store(true)
		}
		p.mu.Unlock()
	}
	p.signal()
	return p.status(r.Context())
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
func (s *Server) bindUsage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	in.Code = strings.TrimSpace(in.Code)
	if !usageCodePattern.MatchString(in.Code) {
		return nil, bad(usageInvalidCode)
	}
	if err := s.usage.perform(r.Context(), "bind", in.Code, r, u); err != nil {
		return nil, err
	}
	return s.usage.status(r.Context())
}
func (s *Server) unbindUsage(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct{}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if err := s.usage.perform(r.Context(), "unbind", "", r, u); err != nil {
		return nil, err
	}
	return s.usage.status(r.Context())
}

// begin serializes only outbound telemetry calls. The generation context makes
// an opt-out cancel both network requests and their subsequent local writes.
func (p *usageReporter) begin(parent context.Context) (context.Context, func(), error) {
	p.outboundMu.Lock()
	p.mu.Lock()
	if !p.enabled.Load() || p.stopped {
		p.mu.Unlock()
		p.outboundMu.Unlock()
		return nil, nil, bad(usageNotEnabled)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.origin+"/api/v1/instances/"+path, bytes.NewReader(raw))
	if err != nil {
		return out, bad(usageUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Monitor-Go/"+usageVersion(buildinfo.Version))
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
	if out.Account != nil && !usageLoginPattern.MatchString(out.Account.Login) {
		return out, &apiError{502, usageUnavailable}
	}
	return out, nil
}
func accountLogin(result usageResult) string {
	if result.Account != nil {
		return result.Account.Login
	}
	return ""
}
func (p *usageReporter) perform(parent context.Context, action, code string, r *http.Request, u *User) error {
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
			return p.failed(ctx, action, st.Failures, err)
		}
	}
	var days []string
	var payload any
	if action == "report" {
		days, err = p.days(ctx)
		if err != nil {
			return err
		}
		payload = p.payload(st.InstanceID, days)
	} else {
		fields := map[string]string{"instanceId": st.InstanceID}
		if action == "bind" {
			fields["code"] = code
		}
		payload = fields
	}
	result, err := p.post(ctx, action, credential, payload)
	var remote *usageRemoteError
	if errors.As(err, &remote) && remote.status == http.StatusNotFound {
		// The console can expire a long-inactive registration. Recreate it with the
		// same durable identity and credential, then retry exactly once. A wrong
		// credential (401/403) is never silently replaced or retried as registration.
		err = p.write(ctx, func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, "UPDATE usage_settings SET registered=0,account_login='' WHERE id=1 AND enabled=1")
			return e
		})
		if err == nil {
			err = p.register(ctx, st, credential)
		}
		if err == nil {
			result, err = p.post(ctx, action, credential, payload)
		}
	}
	if err != nil {
		return p.failed(ctx, action, st.Failures, err)
	}
	if result.InstanceID != st.InstanceID {
		return p.failed(ctx, action, st.Failures, &apiError{502, usageUnavailable})
	}
	return p.write(ctx, func(tx *sql.Tx) error {
		if action == "report" {
			when := time.Now().Unix()
			if _, e := tx.ExecContext(ctx, `UPDATE usage_settings SET last_report_at=?,next_report_at=?,last_error='',failures=0,account_login=? WHERE id=1 AND enabled=1`, when, when+usageReportDelay(), accountLogin(result)); e != nil {
				return e
			}
			for _, day := range days {
				if _, e := tx.ExecContext(ctx, "DELETE FROM usage_activity WHERE day=?", day); e != nil {
					return e
				}
			}
			_, e := tx.ExecContext(ctx, "DELETE FROM usage_activity WHERE day<?", time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02"))
			return e
		}
		if _, e := tx.ExecContext(ctx, "UPDATE usage_settings SET account_login=? WHERE id=1 AND enabled=1", accountLogin(result)); e != nil {
			return e
		}
		audit := "Usage account bound"
		if action == "unbind" {
			audit = "Usage account unbound"
		}
		return auditRequest(tx, r, u.Name, audit, "usage")
	})
}
func (p *usageReporter) register(ctx context.Context, st usageState, credential string) error {
	payload := p.payload(st.InstanceID, nil)
	registration := struct {
		InstanceID string `json:"instanceId"`
		Credential string `json:"credential"`
		Version    string `json:"version"`
		OS         string `json:"os"`
		Arch       string `json:"arch"`
		Deployment string `json:"deployment"`
	}{payload.InstanceID, credential, payload.Version, payload.OS, payload.Arch, payload.Deployment}
	result, e := p.post(ctx, "register", "", registration)
	if e != nil {
		return e
	}
	if result.InstanceID != st.InstanceID {
		return &apiError{502, usageUnavailable}
	}
	err := p.write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "UPDATE usage_settings SET registered=1,account_login=? WHERE id=1 AND enabled=1", accountLogin(result))
		return e
	})
	if err != nil {
		return err
	}

	return nil
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
func (p *usageReporter) failed(ctx context.Context, action string, failures int, cause error) error {
	if ctx.Err() != nil {
		return &apiError{409, usageChanged}
	}
	if action != "report" {
		return cause
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
			if err := p.perform(ctx, "report", "", nil, nil); err != nil {
				retryNotBefore = time.Now().Add(time.Minute)
			} else {
				retryNotBefore = time.Time{}
			}
		}
	}
}
