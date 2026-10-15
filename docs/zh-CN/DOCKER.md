# Docker 部署

[English](../en-US/DOCKER.md) · **简体中文** · [日本語](../ja-JP/DOCKER.md) · [한국어](../ko-KR/DOCKER.md) · [繁體中文](../zh-TW/DOCKER.md) · [Documentation](../README.md)

使用 Docker Engine / Docker Desktop 的 Linux 容器模式、BuildKit 和 Docker Compose v2.20 或更新版本。镜像从当前源码构建，`monitor-go:local` 是本地标签，不是已经发布的仓库镜像。通过 Docker 构建和运行程序时，宿主机不需要安装 Go 或 Node.js。


`make docker-build` 会注入 Git 派生版本。直接使用 Compose 时可传入 `MONITOR_VERSION`，未指定则显示 `0.4.0-dev`。直接构建可用 `docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || printf 0.4.0-dev)" -t monitor-go:local .`。Git 历史不进入 Docker 构建上下文。

## 构建与初始化

```sh
git clone https://github.com/lorvanto/monitor-go.git
cd monitor-go
docker compose up -d --build
docker compose exec monitor cat /data/setup-token
```

打开 [本地服务](http://127.0.0.1:5080/)，使用读取到的初始化码创建首个管理员。首次界面默认英文，初始化后删除码文件。Compose 默认仅绑定 `127.0.0.1`。如果 5080 已被占用，可换一个宿主机端口：

```sh
MONITOR_PORT=5081 docker compose up -d --build
```

## 数据与运行边界

命名卷 `monitor-data` 持久保存整个 `/data`，包括 SQLite 及伴随文件、`master.key`、初始化状态和上传文件，与源码目录中的 `data/` 独立。一个数据卷只运行一个服务进程。`docker compose down` 保留数据卷；`docker compose down -v` 会删除数据卷。重建容器或升级镜像会复用原卷。

容器使用 UID/GID `10001:10001`，Compose 配置只读根文件系统，可写区域为 `/data` 和临时 `/tmp`，以 `/readyz` 检查就绪状态。首次创建命名卷时继承已准备的目录权限。使用自定义 bind mount 时，先创建 UID/GID 10001 可写、权限 0700 的专用目录，不能挂载正在运行的原生实例目录。

默认关闭容器内本机指标采集。在真实主机（包括 Docker 宿主机）上运行原生 Agent，并从界面接入。可选 `MONITOR_COLLECT_HOST=true` 只能采集容器或虚拟机环境，不保证是真实物理机指标。服务探测在容器内执行，`127.0.0.1` 指向容器自身，应使用可访问的服务 DNS/IP；Docker Desktop 可通过 `host.docker.internal` 访问宿主机服务。不需要 Docker socket、宿主机 PID 命名空间或 privileged 模式。

## 配置与 HTTPS

Compose 读取 shell 环境变量或本机未跟踪的 `.env`：`MONITOR_IMAGE`（默认 `monitor-go:local`）、`MONITOR_PORT`（5080）、`MONITOR_BIND_ADDRESS`（127.0.0.1）、`MONITOR_PUBLIC_URL`（空）、`MONITOR_SECURE_COOKIE`（false）、`MONITOR_COLLECT_HOST`（false）及 `MONITOR_TRUSTED_PROXY`（空）。后续 Compose 命令保持相同配置。容器内部端口保持 5080，以便健康检查正常运行。

远程浏览器和 Agent 访问应在宿主机端口前部署可信 HTTPS 反向代理，并配置不带末尾斜杠的实际外部来源：

```sh
MONITOR_PUBLIC_URL=https://monitor.example.com MONITOR_SECURE_COOKIE=true \
  docker compose up -d --build
```

该示例假设代理运行在宿主机。如需其他机器访问映射端口，明确设置 `MONITOR_BIND_ADDRESS`。私有 CA 还需安装或挂载到容器信任库，容器不会自动继承宿主机证书配置。

## 不使用 Compose

```sh
docker build -t monitor-go:local .
docker volume create monitor-go-data
docker run -d --name monitor-go --restart unless-stopped \
  -p 127.0.0.1:5080:5080 --mount source=monitor-go-data,target=/data \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --cap-drop ALL --security-opt no-new-privileges:true \
  --stop-timeout 20 monitor-go:local
docker exec monitor-go cat /data/setup-token
```

## 停机备份与恢复

先停止写入，再归档整个数据卷。临时辅助容器使用相同的非 root 运行用户读取停机数据。备份包含凭据和业务资料，应私密保存。

```sh
mkdir -p backups
docker compose stop monitor
docker compose run --rm --no-deps -T --entrypoint tar monitor \
  -C /data -czf - . > backups/monitor-go-data.tar.gz
docker compose start monitor
```

恢复到全新的 Compose 项目和空卷。命令拒绝覆盖已有数据，并以运行用户（UID/GID 10001）创建恢复文件。换到新 Docker 主机时先构建兼容的本地镜像。不要把原实例和恢复实例同时作为同一套安装运行。

```sh
docker compose -p monitor-go-restore run --rm --no-deps -T \
  --entrypoint sh monitor -c \
  'test -z "$(ls -A /data)" && tar -xzf - -C /data' \
  < backups/monitor-go-data.tar.gz
MONITOR_PORT=5081 docker compose -p monitor-go-restore up -d --no-build
```

## 升级与回退

升级前先完成并验证停机备份，然后更新审查过的源码并重新构建：

```sh
git pull --ff-only
docker compose up -d --build
docker compose ps
docker compose logs --tail=100 monitor
```

保持 Compose 项目名和数据卷一致，升级时不删除卷。SQLite 在启动时执行版本迁移。回退需要旧镜像或源码和升级前的完整数据卷备份；旧程序可能无法读取升级后的数据库。

## 验证与镜像维护

冒烟测试另需测试机器安装 Node.js 22.12+ / 24.x。脚本使用临时 Compose 项目和数据卷，自动选择回环端口，验证初始化、权限、健康检查、上传字节及 SHA-256、SQLite/会话持久化、容器重建和停机备份恢复。结束后清理自身测试项目，保留本地源码镜像。

```sh
docker compose config --quiet
node scripts/test-docker.mjs
```

指定测试架构时设置 `MONITOR_PLATFORM=linux/amd64` 或 `linux/arm64`，并用独立的 `MONITOR_IMAGE` 标签。运行非本机架构需要 Docker 引擎支持相应模拟器；交叉构建本身不等于运行验证。

构建分为 Node、Go 和 Alpine 阶段，运行镜像不包含编译工具。npm/Go 锁文件与版本化基础镜像锁定构建输入，版本和用途见 [第三方声明](../../THIRD_PARTY_NOTICES.md)。运行时证书和时区资料来自 Alpine，部署时应重新构建并核查基础镜像与系统包。`NODE_IMAGE`、`GO_IMAGE`、`RUNTIME_IMAGE` 构建参数用于经审查的替换。支持 BuildKit 交叉构建，承诺的运行平台仍需分别实测。这些命令不推送镜像仓库、不创建版本标签，也不自动发布。

参考: [Docker 数据卷](https://docs.docker.com/engine/storage/volumes/) · [多平台构建](https://docs.docker.com/build/building/multi-platform/).

## 可信代理与登录限流

Compose 将 `MONITOR_TRUSTED_PROXY` 传给 `-trusted-proxy`；直连时保持为空。按容器实际看到的代理地址配置 CIDR，不要随意信任整个私网。例如，已确认代理地址为 `172.20.0.2` 时使用 `MONITOR_TRUSTED_PROXY=172.20.0.2/32`。宿主机到容器的转发可能改变来源地址，应先核实网络再启用信任。

`-trusted-proxy` 接受逗号分隔的代理 CIDR，默认留空（不信任代理），非法值会阻止启动。只填写本进程实际看到的代理地址；代理必须覆盖或安全追加 `X-Forwarded-For`。服务端从右向左寻找首个不可信地址，忽略其他来源提供的转发头。登录只统计失败：每个客户端 IP 每 5 分钟 30 次、每个规范化用户名每 5 分钟 10 次；成功登录不消耗也不重置这些计数。初始化对所有尝试计数（每个 IP 每 5 分钟 10 次）。

## 在线备份与恢复

执行命令时服务可以继续运行。输出目录必须不存在，父目录必须已存在。命令只读打开源库，不获取实例锁、不执行迁移（支持结构版本 3 至当前版本），生成一致的 SQLite 快照，并复制主密钥及快照引用的全部对象，包括回收站文件。快照之后的新上传不纳入本次备份。缺失对象会重试；仍缺失、损坏或变更时以非零状态退出并保留 BACKUP_INCOMPLETE，不能恢复该目录。完成清单记录 UTC 时间、构建/结构版本、对象数量及 SHA-256。初始化码、锁、暂存文件和图片缓存不复制。

容器命令先在数据卷中生成新快照，随后应立即复制到独立存储。每次使用唯一目录名。示例归档可按下方新数据卷恢复流程还原。

```sh
mkdir -p backups
docker compose exec monitor mkdir -p /data/backups
docker compose exec monitor monitor backup -data /data -out /data/backups/snapshot-001
docker compose exec -T monitor tar -C /data/backups/snapshot-001 -czf - . > backups/monitor-go-online.tar.gz
```

恢复时先停服，用完整备份替换整个数据目录，再使用支持该结构版本或更高版本的程序启动。验证成功前保留旧目录。只支持离线恢复，不要向运行中的实例合并文件。备份包含凭据，应私密保存并复制到独立存储；原有冷备份方式仍可使用。
