package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/lorvanto/monitor-go/internal/agent"
	"github.com/lorvanto/monitor-go/internal/buildinfo"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	server := flag.String("server", "", "服务地址，远程必须使用 HTTPS")
	enrollment := flag.String("enrollment-file", "", "首次运行的接入码文件（请设为 600 权限）")
	config := flag.String("config", "agent-data/config.json", "持久凭据配置文件")
	ca := flag.String("ca-file", "", "可选私有 CA PEM 文件")
	version := flag.Bool("version", false, "Print application version and exit")
	flag.Parse()
	if *version {
		fmt.Println(buildinfo.Version)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if e := agent.Run(ctx, *config, *server, *enrollment, *ca); e != nil {
		slog.Error("Agent 已停止", "error", e)
		os.Exit(1)
	}
}
