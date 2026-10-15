package app

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

func forbiddenIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.String() == "100.100.100.200" || ip.String() == "fd00:ec2::254"
}
func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil {
		return nil, e
	}
	if len(ips) == 0 {
		return nil, errors.New("未找到目标地址")
	}
	for _, ip := range ips {
		if forbiddenIP(ip) {
			return nil, errors.New("不允许探测链路本地、元数据或组播地址")
		}
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	var last error
	for _, ip := range ips {
		c, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return c, nil
		}
		last = e
	}
	return nil, last
}
func init() {
	mysql.RegisterDialContext("monitor-safe", func(ctx context.Context, addr string) (net.Conn, error) { return safeDial(ctx, "tcp", addr) })
}
func probe(ctx context.Context, m Monitor, password string) CheckResult {
	start := time.Now()
	result := CheckResult{At: now(), State: "healthy"}
	var err error
	switch m.Type {
	case "HTTP":
		tr := &http.Transport{DialContext: safeDial, TLSHandshakeTimeout: time.Duration(m.Timeout) * time.Second, DisableKeepAlives: true}
		defer tr.CloseIdleConnections()
		client := &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("重定向次数过多")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("重定向协议不支持")
			}
			if req.URL.User != nil {
				return errors.New("重定向不允许登录凭据")
			}
			return nil
		}}
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, m.Target, nil)
		if e != nil {
			err = e
			break
		}
		req.Header.Set("User-Agent", buildinfo.UserAgent())
		resp, e := client.Do(req)
		if e != nil {
			err = e
			break
		}
		io.CopyN(io.Discard, resp.Body, 4096)
		resp.Body.Close()
		result.Detail = fmt.Sprintf("HTTP %d · 预期 %d", resp.StatusCode, m.Expected)
		if resp.StatusCode != m.Expected {
			result.State = "failed"
		}
	case "TCP":
		c, e := safeDial(ctx, "tcp", m.Target)
		if e != nil {
			err = e
			break
		}
		c.Close()
		result.Detail = "TCP 连接成功"
	case "SSL":
		c, e := safeDial(ctx, "tcp", m.Target)
		if e != nil {
			err = e
			break
		}
		defer c.Close()
		host, _, _ := net.SplitHostPort(m.Target)
		tc := tls.Client(c, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		err = tc.HandshakeContext(ctx)
		if err != nil {
			break
		}
		cert := tc.ConnectionState().PeerCertificates[0]
		days := time.Until(cert.NotAfter).Hours() / 24
		result.Detail = fmt.Sprintf("证书有效至 %s，剩余 %.1f 天", cert.NotAfter.UTC().Format(time.RFC3339), days)
		if days <= float64(m.SSLDays) {
			result.State = "warning"
		}
	case "MySQL":
		cfg := mysql.NewConfig()
		cfg.User = m.Username
		cfg.Passwd = password
		cfg.Net = "monitor-safe"
		cfg.Addr = m.Target
		cfg.Timeout = time.Duration(m.Timeout) * time.Second
		cfg.ReadTimeout = cfg.Timeout
		cfg.WriteTimeout = cfg.Timeout
		if m.TLS {
			cfg.TLSConfig = "true"
		}
		connector, e := mysql.NewConnector(cfg)
		if e != nil {
			err = e
			break
		}
		db := sql.OpenDB(connector)
		defer db.Close()
		db.SetMaxOpenConns(1)
		var one int
		err = db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
		result.Detail = "MySQL SELECT 1 成功"
	case "Redis":
		c, e := safeDial(ctx, "tcp", m.Target)
		if e != nil {
			err = e
			break
		}
		defer c.Close()
		deadline, _ := ctx.Deadline()
		c.SetDeadline(deadline)
		if m.TLS {
			host, _, _ := net.SplitHostPort(m.Target)
			tc := tls.Client(c, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
			if e = tc.HandshakeContext(ctx); e != nil {
				err = e
				break
			}
			c = tc
		}
		br := bufio.NewReader(io.LimitReader(c, 4096))
		command := func(args ...string) (string, error) {
			var b strings.Builder
			fmt.Fprintf(&b, "*%d\r\n", len(args))
			for _, v := range args {
				fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(v), v)
			}
			if _, e := io.WriteString(c, b.String()); e != nil {
				return "", e
			}
			line, e := br.ReadString('\n')
			if e != nil {
				return "", e
			}
			return strings.TrimSpace(line), nil
		}
		if password != "" {
			args := []string{"AUTH", password}
			if m.Username != "" {
				args = []string{"AUTH", m.Username, password}
			}
			line, e := command(args...)
			if e != nil {
				err = e
				break
			}
			if line != "+OK" {
				err = errors.New("Redis 认证失败")
				break
			}
		}
		line, e := command("PING")
		if e != nil {
			err = e
			break
		}
		if line != "+PONG" {
			err = errors.New("Redis 未返回 PONG")
			break
		}
		result.Detail = "Redis PONG"
	default:
		err = errors.New("检测类型不支持")
	}
	result.Latency = float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		result.State = "failed"
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			result.Detail = "检测超时"
		} else {
			result.Detail = err.Error()
			if password != "" {
				result.Detail = strings.ReplaceAll(result.Detail, password, "[已隐藏]")
			}
			if len(result.Detail) > 1000 {
				result.Detail = result.Detail[:1000]
			}
		}
	}
	return result
}
