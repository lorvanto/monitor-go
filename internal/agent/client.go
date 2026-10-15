package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/flock"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"github.com/lorvanto/monitor-go/internal/hoststats"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Server          string `json:"server"`
	Credential      string `json:"credential"`
	EnrollmentToken string `json:"enrollmentToken,omitempty"`
	CAFile          string `json:"caFile,omitempty"`
}

func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func ValidateServer(server string) error {
	u, e := url.Parse(server)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("服务地址必须为完整来源，例如 https://monitor.example.com")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("远程接入必须使用 HTTPS；仅回环地址允许 HTTP 联调")
}
func save(path string, c Config) error {
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".agent-config-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, path)
}
func LoadOrCreate(path, server, enrollmentFile, caFile string) (Config, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e == nil {
		info, err := os.Stat(path)
		if err != nil {
			return c, err
		}
		if info.Mode().Perm()&0077 != 0 {
			return c, errors.New("Agent 配置包含凭据，请先设置 chmod 600")
		}
		e = json.Unmarshal(b, &c)
	} else if errors.Is(e, os.ErrNotExist) {
		if server == "" || enrollmentFile == "" {
			return c, errors.New("首次运行需要 -server 与 -enrollment-file")
		}
		if e = ValidateServer(server); e != nil {
			return c, e
		}
		info, err := os.Stat(enrollmentFile)
		if err != nil {
			return c, err
		}
		if info.Mode().Perm()&0077 != 0 {
			return c, errors.New("接入码文件包含凭据，请先设置 chmod 600")
		}
		if info.Size() > 128 {
			return c, errors.New("接入码文件过大")
		}
		b, e = os.ReadFile(enrollmentFile)
		if e != nil {
			return c, e
		}
		c = Config{Server: strings.TrimRight(server, "/"), Credential: token(), EnrollmentToken: strings.TrimSpace(string(b)), CAFile: caFile}
		decoded, err := hex.DecodeString(c.EnrollmentToken)
		if err != nil || len(decoded) != 32 {
			return c, errors.New("接入码格式不正确")
		}
		if e = os.MkdirAll(filepath.Dir(path), 0700); e == nil {
			e = save(path, c)
		}
	}
	if e != nil {
		return c, e
	}
	if e = ValidateServer(c.Server); e != nil {
		return c, e
	}
	key, e := hex.DecodeString(c.Credential)
	if e != nil || len(key) != 32 {
		return c, errors.New("Agent 配置凭据格式不正确")
	}
	c.Server = strings.TrimRight(c.Server, "/")
	return c, nil
}
func NewClient(c Config) (*http.Client, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		b, e := os.ReadFile(c.CAFile)
		if e != nil {
			return nil, e
		}
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, errors.New("CA 文件无有效证书")
		}
		tc.RootCAs = pool
	}
	return &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: nil, TLSClientConfig: tc, TLSHandshakeTimeout: 5 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Agent API 不允许重定向") }}, nil
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("服务端返回 HTTP %d", e.Status) }
func Send(ctx context.Context, client *http.Client, c Config, path string, body any, authenticated bool) error {
	b, e := json.Marshal(body)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.Server+"/api/v1/agent/"+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+c.Credential)
	}
	res, e := client.Do(req)
	if e != nil {
		return errors.New("无法连接服务端，请检查地址、网络和 TLS 证书")
	}
	defer res.Body.Close()
	io.CopyN(io.Discard, res.Body, 4096)
	if res.StatusCode != http.StatusOK {
		return &HTTPError{res.StatusCode}
	}
	return nil
}

type runDependencies struct {
	collect func(context.Context) (hoststats.Report, error)
	wait    func(context.Context, time.Duration) error
}

func waitForReport(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func Run(ctx context.Context, path, server, enrollmentFile, caFile string) error {
	return run(ctx, path, server, enrollmentFile, caFile, runDependencies{})
}
func run(ctx context.Context, path, server, enrollmentFile, caFile string, deps runDependencies) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	lock := flock.New(path + ".lock")
	ok, e := lock.TryLock()
	if e != nil {
		return e
	}
	if !ok {
		return errors.New("相同配置的 Agent 已经运行")
	}
	defer lock.Unlock()
	c, e := LoadOrCreate(path, server, enrollmentFile, caFile)
	if e != nil {
		return e
	}
	client, e := NewClient(c)
	if e != nil {
		return e
	}
	defer client.CloseIdleConnections()
	collector := hoststats.Collector{}
	if deps.collect == nil {
		deps.collect = collector.Collect
	}
	if deps.wait == nil {
		deps.wait = waitForReport
	}
	delay := time.Duration(0)
	failures := 0
	for {
		if deps.wait(ctx, delay) != nil {
			return nil
		}
		if c.EnrollmentToken != "" {
			e = Send(ctx, client, c, "enroll", map[string]string{"token": c.EnrollmentToken, "credential": c.Credential}, false)
			if e == nil {
				c.EnrollmentToken = ""
				if e = save(path, c); e != nil {
					return e
				}
				slog.Info("Agent 注册完成")
				delay = 0
				failures = 0
				continue
			}
		} else {
			var report hoststats.Report
			report, e = deps.collect(ctx)
			if e == nil {
				report.ReportID = token()
				report.Inventory.Source = "远程 Agent"
				e = Send(ctx, client, c, "reports", report, true)
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if e != nil {
			var he *HTTPError
			if errors.As(e, &he) && (he.Status == 401 || he.Status == 403 || he.Status == 409 && c.EnrollmentToken != "") {
				return errors.New("接入凭据失效或已撤销，请在主机页面重新接入并使用新的配置文件")
			}
			failures++
			delay = time.Duration(min(failures*5, 60)) * time.Second
			slog.Warn("Agent 上报未成功，将重试", "error", e, "retry_seconds", delay.Seconds())
		} else {
			failures = 0
			delay = 15 * time.Second
		}
	}
}
