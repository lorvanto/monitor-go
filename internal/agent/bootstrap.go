package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/lorvanto/monitor-go/internal/hoststats"
)

// Bootstrap registers if needed and confirms one accepted report, then exits.
// The credential is persisted before enrollment so an interrupted installation
// can safely repeat the request without consuming the code with a new key.
func Bootstrap(ctx context.Context, path, server, enrollmentFile, caFile string) error {
	return bootstrap(ctx, path, server, enrollmentFile, caFile, runDependencies{})
}

func bootstrap(ctx context.Context, path, server, enrollmentFile, caFile string, deps runDependencies) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := flock.New(path + ".lock")
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("相同配置的 Agent 已经运行")
	}
	defer lock.Unlock()
	c, err := LoadOrCreate(path, server, enrollmentFile, caFile)
	if err != nil {
		return err
	}
	if server != "" && strings.TrimRight(server, "/") != c.Server {
		return errors.New("现有 Agent 配置属于其他服务，请使用新的接入码明确替换")
	}
	client, err := NewClient(c)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	collector := hoststats.Collector{}
	if deps.collect == nil {
		deps.collect = collector.Collect
	}
	if deps.wait == nil {
		deps.wait = waitForReport
	}
	report := func() error {
		for attempt := 0; ; attempt++ {
			v, err := deps.collect(ctx)
			if err != nil {
				return err
			}
			v.ReportID = token()
			v.Inventory.Source = "远程 Agent"
			err = Send(ctx, client, c, "reports", v, true)
			var status *HTTPError
			if attempt != 0 || !errors.As(err, &status) || status.Status != 429 {
				return err
			}
			// A repeated installer can follow the previous report immediately.
			if err = deps.wait(ctx, 5*time.Second); err != nil {
				return err
			}
		}
	}
	if c.EnrollmentToken == "" {
		return report()
	}
	err = Send(ctx, client, c, "enroll", map[string]string{"token": c.EnrollmentToken, "credential": c.Credential}, false)
	if err != nil {
		// The enrollment may have committed before its response was lost. A
		// report also recovers that state after the short-lived code expires.
		if reportErr := report(); reportErr != nil {
			return err
		}
		c.EnrollmentToken = ""
		return save(path, c)
	}
	c.EnrollmentToken = ""
	if err = save(path, c); err != nil {
		return err
	}
	return report()
}
