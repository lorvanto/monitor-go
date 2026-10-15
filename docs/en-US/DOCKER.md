# Docker deployment

**English** · [简体中文](../zh-CN/DOCKER.md) · [日本語](../ja-JP/DOCKER.md) · [한국어](../ko-KR/DOCKER.md) · [繁體中文](../zh-TW/DOCKER.md) · [Documentation](../README.md)

Use Linux containers with Docker Engine / Docker Desktop, BuildKit and Docker Compose v2.20 or newer. Build from this checkout; `monitor-go:local` is a local tag, not a published registry image. No host Go or Node.js installation is needed to build and run the application through Docker.


`make docker-build` injects the Git-derived version. With plain Compose, supply `MONITOR_VERSION` explicitly; without it, the image reports `0.4.0-dev`. For direct builds use `docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || printf 0.4.0-dev)" -t monitor-go:local .`. Git history stays outside the Docker build context.

## Build and initialize

```sh
git clone https://github.com/lorvanto/monitor-go.git
cd monitor-go
docker compose up -d --build
docker compose exec monitor cat /data/setup-token
```

Open [the local service](http://127.0.0.1:5080/) and create the first administrator with that setup token. The UI starts in English on first use. The token file is deleted after setup. Compose binds only to `127.0.0.1` by default. If 5080 is occupied, use a different host port:

```sh
MONITOR_PORT=5081 docker compose up -d --build
```

## Data and runtime behavior

The `monitor-data` named volume stores all of `/data`: SQLite and companion files, `master.key`, setup state and uploads. It is separate from the checkout’s `data/` directory. Keep one service process per volume. `docker compose down` preserves the volume; `docker compose down -v` deletes it. Container recreation and image upgrades reuse the same volume.

The runtime uses UID/GID `10001:10001`, a read-only root filesystem in Compose, writable `/data` and temporary `/tmp`, and a `/readyz` health check. Named volumes receive the prepared directory ownership. For a custom bind mount, create a dedicated directory writable by UID/GID 10001 with mode 0700; do not point it at a running native instance.

Container-local host collection is disabled by default. Install a native Agent on each real host, including the Docker host, and enroll it through the UI. Optional `MONITOR_COLLECT_HOST=true` observes the container/VM environment, not guaranteed physical-host metrics. Service checks run inside the container: `127.0.0.1` refers to the container. Use reachable service DNS/IPs; Docker Desktop provides `host.docker.internal` for host services. No Docker socket, host PID namespace or privileged mode is required.

## Settings and HTTPS

Compose reads these shell variables (or a local, untracked `.env`): `MONITOR_IMAGE` (default `monitor-go:local`), `MONITOR_PORT` (5080), `MONITOR_BIND_ADDRESS` (127.0.0.1), `MONITOR_PUBLIC_URL` (empty), `MONITOR_SECURE_COOKIE` (false), and `MONITOR_COLLECT_HOST` (false), plus `MONITOR_TRUSTED_PROXY` (empty). Preserve these values on later Compose commands. Keep the container port at 5080 for the health check.

For remote browser and Agent access, place a trusted HTTPS proxy in front of the host port and set the exact external origin without a trailing slash:

```sh
MONITOR_PUBLIC_URL=https://monitor.example.com MONITOR_SECURE_COOKIE=true \
  docker compose up -d --build
```

The example assumes a proxy running on the host. Set `MONITOR_BIND_ADDRESS` deliberately if the host port must be reachable elsewhere. Private certificate authorities must also be installed or mounted into the container trust store; host trust configuration is not automatically inherited.

## Without Compose

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

## Cold backup and restore

Stop writes and archive the entire volume. The temporary helper reads the stopped data as the same non-root runtime user. Treat the archive as private credentials and operational data.

```sh
mkdir -p backups
docker compose stop monitor
docker compose run --rm --no-deps -T --entrypoint tar monitor \
  -C /data -czf - . > backups/monitor-go-data.tar.gz
docker compose start monitor
```

Restore to a fresh Compose project and empty volume. The command refuses to overwrite existing data, and creates restored files as the runtime user (UID/GID 10001). Build the compatible local image first on a new Docker host. Do not run the source and restored instances as one installation at the same time.

```sh
docker compose -p monitor-go-restore run --rm --no-deps -T \
  --entrypoint sh monitor -c \
  'test -z "$(ls -A /data)" && tar -xzf - -C /data' \
  < backups/monitor-go-data.tar.gz
MONITOR_PORT=5081 docker compose -p monitor-go-restore up -d --no-build
```

## Upgrade and rollback

Take and verify a cold backup before changing versions. Then update the reviewed source and rebuild:

```sh
git pull --ff-only
docker compose up -d --build
docker compose ps
docker compose logs --tail=100 monitor
```

Keep the same Compose project name and volume. Never remove the volume during upgrades. SQLite migrations run on startup. Rollback requires the prior image/source and the complete pre-upgrade volume backup; an older binary may not read an upgraded schema.

## Validation and image maintenance

The smoke test additionally requires Node.js 22.12+ / 24.x on the test machine. It uses disposable Compose projects and volumes on an automatically selected loopback port; it checks initialization, permissions, health, upload bytes/SHA-256, SQLite/session persistence, recreation and cold backup/restore. It cleans up its own test projects and leaves the source-built image locally.

```sh
docker compose config --quiet
node scripts/test-docker.mjs
```

For an explicit test target, set `MONITOR_PLATFORM=linux/amd64` or `linux/arm64` and use a separate `MONITOR_IMAGE` tag. Non-native execution requires an emulator supported by the Docker engine; a cross-build alone is not a runtime test.

Node, Go and Alpine stages keep build tools out of the runtime. npm/Go locks and versioned base images pin the build inputs; base versions and purposes are recorded in [third-party notices](../../THIRD_PARTY_NOTICES.md). Runtime certificates and time zones come from Alpine. Rebuild and review base/OS packages when deploying. Build arguments `NODE_IMAGE`, `GO_IMAGE` and `RUNTIME_IMAGE` permit reviewed replacements. BuildKit cross-compilation is supported; runtime platforms require separate testing. These commands do not push a registry image, create release tags or publish automatically.

References: [Docker volumes](https://docs.docker.com/engine/storage/volumes/) · [Multi-platform builds](https://docs.docker.com/build/building/multi-platform/).

## Trusted proxies and login limits

Compose passes `MONITOR_TRUSTED_PROXY` to `-trusted-proxy`; leave it empty for direct access. Set it to the actual proxy CIDRs as seen inside the container, not an arbitrary whole private network. For example, a known proxy address `172.20.0.2` uses `MONITOR_TRUSTED_PROXY=172.20.0.2/32`. Host-to-container forwarding may change the peer address: verify your network before enabling trust.

`-trusted-proxy` accepts comma-separated proxy CIDRs and defaults to empty (no trusted proxies). Invalid CIDRs prevent startup. Trust only the actual proxy addresses as seen by this process; the proxy must overwrite or safely append `X-Forwarded-For`. The server walks that header from right to left, stopping at the first untrusted address. Headers from other peers are ignored. Login counts only failures: 30 per client IP and 10 per normalized username per five minutes; successful logins neither consume nor reset those budgets. Setup counts every attempt (10 per IP per five minutes).

## Online backup and restore

The service may keep running during this command. The output directory must not exist and its parent must exist. The command opens the source read-only without acquiring the instance lock or migrating it (supported schemas: 3 through the current version). It creates a consistent SQLite snapshot and copies the master key and all objects referenced by that snapshot, including trash. New uploads after the snapshot are excluded. Missing objects are retried; missing, corrupt or changed objects fail with a nonzero exit status and retain BACKUP_INCOMPLETE. Never restore such a directory. The completion manifest records UTC time, build/schema versions, object count and SHA-256 hashes. Setup tokens, locks, staging and image caches are excluded.

The container command writes a new snapshot inside the data volume; copy it out immediately to independent storage. Choose a unique directory name each time. The example archive can use the fresh-volume restore procedure below.

```sh
mkdir -p backups
docker compose exec monitor mkdir -p /data/backups
docker compose exec monitor monitor backup -data /data -out /data/backups/snapshot-001
docker compose exec -T monitor tar -C /data/backups/snapshot-001 -czf - . > backups/monitor-go-online.tar.gz
```

To restore, stop the service, replace the entire data directory with a complete backup, and start a binary supporting at least its schema version. Keep the old directory until verification succeeds. Restore offline only; do not merge files into a running installation. Treat backups as private credentials and copy them to independent storage. Cold backups remain supported.
