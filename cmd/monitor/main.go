package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/app"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"github.com/lorvanto/monitor-go/web"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		if err := runBackup(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "Backup failed:", err)
			os.Exit(1)
		}
		return
	}
	listen := flag.String("listen", "127.0.0.1:5080", "HTTP 监听地址")
	data := flag.String("data", "data", "SQLite 和密钥目录")
	public := flag.String("public-url", "", "浏览器访问的完整来源，例如 https://monitor.example.com")
	secure := flag.Bool("secure-cookie", false, "HTTPS 部署时启用 Secure Cookie")
	collect := flag.Bool("collect-host", true, "采集运行服务的本机指标")
	trustedProxy := flag.String("trusted-proxy", "", "Comma-separated trusted proxy CIDRs for X-Forwarded-For; empty trusts none")
	version := flag.Bool("version", false, "Print application version and exit")
	flag.Parse()
	if *version {
		fmt.Println(buildinfo.Version)
		return
	}
	s, e := app.New(app.Config{Listen: *listen, DataDir: *data, PublicURL: *public, TrustedProxy: *trustedProxy, SecureCookie: *secure, Collect: *collect, Assets: web.Assets()})
	if e != nil {
		slog.Error("启动失败", "error", e)
		os.Exit(1)
	}
	defer s.Store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: *listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	s.Start(ctx)
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(c)
	}()
	slog.Info("监控服务启动", "address", *listen, "data", s.Store.Dir)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		stop()
		s.Wait()
		slog.Error("HTTP 服务失败", "error", e)
		os.Exit(1)
	}
	stop()
	s.Wait()
}

func runBackup(args []string) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	source := flags.String("data", "data", "Source data directory (service may stay running)")
	output := flags.String("out", "", "New backup directory; parent must exist")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" || flags.NArg() != 0 {
		return fmt.Errorf("usage: monitor backup -data <directory> -out <new-directory>")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manifest, err := app.Backup(ctx, *source, *output)
	if err != nil {
		return err
	}
	fmt.Printf("Backup complete: %s (schema %d, %d objects)\n", *output, manifest.SchemaVersion, manifest.FileCount)
	return nil
}
