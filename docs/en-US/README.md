# Monitor Go — user guide

**English** · [简体中文](../zh-CN/README.md) · [日本語](../ja-JP/README.md) · [한국어](../ko-KR/README.md) · [繁體中文](../zh-TW/README.md) · [Documentation](../README.md)

> Status: pre-release development baseline `0.4.0`, SQLite schema `5`; no published release is implied. Examples use a POSIX shell; Windows developers can use WSL.

Schema 5 adds a partial index for queued scheduler jobs, avoiding scans of completed-job history. Job retention remains seven days. Stop the service and back up the complete data directory before upgrading; older binaries cannot read schema 5, and rollback requires that backup.

See the [English interface screenshots](../../README.md#screenshots) for a feature tour using synthetic demonstration data. English is the default language on first use.

## Product scope

Go serves a React 19 interface and uses a local SQLite database. A team shares one installation and its resources. Creator fields are for auditing, not data isolation. MySQL and Redis are check targets, not storage requirements.

Implemented features include the overview, local/remote host metrics, HTTP/TCP/SSL/MySQL/Redis checks, incidents, roles and members, SMTP/Webhook delivery, menus, private files, image styles, audit logs, system settings, application keys, packages and QR/barcode generation. Light/dark themes and keyboard page search are available. Permission checks are enforced on the server independently of menu visibility.

## Frontend architecture

The client uses React / React DOM 19.3.0, TypeScript 5.9.3, Vite 8.3.4 and Zustand 5.0.15. It builds static assets embedded in the Go server; no Node.js service is needed in production. Business views in `web/src/views/` load with `React.lazy`; reusable TSX components are in `components/`, shared session/permission/navigation state in `stores/app.ts`, and API types, chart helpers and formatters in `lib/`. The five catalogs remain in `locales/`.

`npm --prefix web test` runs locale tests plus Vitest and React Testing Library interaction tests. `npm --prefix web run typecheck` checks both application and test TypeScript. The React client uses the existing API, CSRF, file-upload and SQLite contracts; this migration adds no database schema or data migration. Rebuild and restart the Go binary to deploy the new embedded client.

The client ignores late responses from earlier sessions and selections. Host-note edits and incident-silence dialogs keep their original resource and version. File pagination recovers when the last page becomes empty, and audit filters can be cleared during loading. Themes still work when preference storage is blocked. Failed page loads offer a reload action with a warning that unsaved changes will be lost.

## Docker deployment

Build and run with Docker without installing Go or Node.js on the host:

```sh
docker compose up -d --build
docker compose exec monitor cat /data/setup-token
```

Open port 5080 and create the administrator with that token. The named volume persists the complete data directory. Container-local host collection is disabled by default; use a native Agent on real hosts. See [Docker deployment](DOCKER.md) for port overrides, HTTPS, backup, restore and upgrades.

## Build and first run

Use Go 1.26.9 or a newer compatible version and Node.js 22.12+ / 24.x. Keep all lockfiles. Build from this repository:

```sh
make build
./bin/monitor -data data -listen 127.0.0.1:5080
```

Visit [localhost:5080](http://127.0.0.1:5080/). Read `data/setup-token` on the server and enter the token, username, name and password in setup. No default username or password is provided. Setup removes the token file. Passwords are 12–72 bytes. Sessions use HttpOnly cookies and CSRF checks; permissions are checked for each API request.

The server binary embeds frontend files. Java, Node.js and external databases are not needed at runtime. Only one active process may use a data directory.

For team access, use a trusted HTTPS reverse proxy:

```sh
./bin/monitor -data data -listen 127.0.0.1:5080 \
  -public-url https://monitor.example.com -secure-cookie
```

Use the exact browser origin without a trailing slash. `-collect-host=false` disables local host collection.

## Interface languages

Select 简体中文, English, 日本語, 한국어 or 繁體中文 on the sign-in/setup page or in the signed-in sidebar. First visits use English, regardless of browser language. A valid saved choice is restored on later visits; missing or invalid preferences and unavailable browser storage use English. Traditional Chinese also matches `zh-Hant`, `zh-HK` and `zh-MO`.

Language switches update fixed UI text, default navigation, permissions, API scopes, dates and numbers without reloading the page or resetting open forms. Date display uses the browser's local time zone. Locale catalogs load on demand. Language is stored locally under `monitor.locale`, not in a shared database setting.

Custom names, notes, uploaded text, saved incident/audit history and probe diagnostics retain their original language. Stored notification bodies and server/agent logs are not translated. Changing language never rewrites these records. Unchanged built-in menu names can be translated; renamed menus remain as entered.

## Monitoring and remote hosts

The service collects local and remote host metrics approximately every 15 seconds. HTTP, TCP, SSL, MySQL and Redis checks run on the monitoring server. Consecutive failures trigger incidents; a subsequent success resolves them. Acknowledging or silencing an incident does not manufacture recovery.

To connect a host, generate a 15-minute enrollment code from Host details. Build an agent matching the destination system:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/agent-linux-amd64 ./cmd/agent
# Use GOARCH=arm64 for ARM64 hosts.
```

Place the downloaded `enrollment-token` and agent binary on the remote host:

```sh
chmod 600 enrollment-token
./agent -server https://monitor.example.com -enrollment-file enrollment-token -config agent-data/config.json
# Subsequent starts:
./agent -config agent-data/config.json
```

Use valid, trusted HTTPS; private certificate authorities can be configured with `-ca-file`. Numeric loopback HTTP is permitted for local tests only. An agent makes outbound requests, so no inbound agent port is needed. Do not copy credentials between hosts. Revocation or re-enrollment invalidates old credentials. Metrics are not buffered during disconnection. Reports absent for over 90 seconds trigger an offline incident on the next check (up to 10 more seconds).

The [systemd template](../../deploy/monitor-go-agent.service) requires the deployer to create its user and data directory. No system service is installed automatically. Agents collect host resources; they do not execute remote commands or service checks.

## Notifications, files and distribution

Configure SMTP or generic Webhook channels in the alert center. SMTP requires TLS or STARTTLS. Webhooks require HTTPS, except numeric loopback for testing. Notifications use persistent jobs with up to four attempts. Delivery is at least once; receivers should deduplicate by `X-MonitorGo-Delivery-ID`. SMTP acceptance does not guarantee inbox delivery. Silence skips pending notifications without later catch-up.

Webhook signatures use `X-MonitorGo-Signature: sha256=<hex HMAC-SHA256>`. Sign the timestamp header, a period, and the original body bytes with the channel secret. Validate signatures and a timestamp window at the receiver.

Files are private and shared within the installation. Upload, preview, download and management have separate permissions. Trash uses storage quota. Permanent deletion requires typing the confirmation phrase shown in the selected language. Originals are stored under `data/uploads/`. Image styles create derivatives without replacing originals.

Create an application to issue scoped keys or distribute packages. Full keys appear only once on creation/rotation; only hashes are stored. Machine integrations use `Authorization: Bearer ...` under `/api/v1/integrations`. They cannot log in to the browser administration API. Host/check/alert scopes cover shared instance resources; package scopes are limited to the key's application.

Publishing a package freezes its application, version, platform, architecture and source. Create a new version to replace a file. External URLs must use HTTPS; never forward an application key to an external download URL. QR codes and barcodes are generated locally and can be exported as PNG/SVG. Input content is not stored.

## Development and verification

Run commands from the repository root. Use an isolated development data directory:

```sh
make build
./bin/monitor -data tmp/development -listen 127.0.0.1:5093
# In another terminal:
MONITOR_GO_API_TARGET=http://127.0.0.1:5093 npm --prefix web run dev
```

The frontend runs on 5188 and this example proxies `/api` to 5093. Without `MONITOR_GO_API_TARGET`, the proxy defaults to 5080. Initialize the isolated instance with its own `tmp/development/setup-token`. Rebuild and restart Go after backend changes.

```sh
make test
npm --prefix web run typecheck
npm --prefix web run check:i18n
npm --prefix web run check:docs
npm --prefix web test
npm --prefix web run format:check
```

`make test` builds frontend assets, runs frontend locale and React interaction tests, Go race tests and vet. API integration tests use actual temporary SQLite files. [Translation maintenance](INTERNATIONALIZATION.md) explains adding languages and preserving business values.

## Backups and upgrades

Stop the service cleanly, then back up the entire data directory: SQLite and any companion files, `master.key`, and original `uploads/` files. Copying only a live `monitor.db` is not a complete backup. Restore the whole directory with a compatible server version.

Database versions 1 → 2 → 3 → 4 → 5 migrate transactionally. Older binaries cannot directly read schema 5; rollback requires the entire pre-upgrade backup. New permissions are not silently assigned to custom roles. This internationalization change requires no new schema migration.

## Current limits

There is no multitenancy, external storage adapter, remote command execution, agent auto-update, online backup UI, public file sharing or proprietary chat-channel templates. Complete editing of existing monitor targets and certificate issuance remain planned. Configured ceilings of 500 checks and 1000 agents are not load-test claims. Production Linux sampling, real email-provider delivery and capacity testing require deployment-specific validation.

Historical design proposals and detailed validation records remain in the [Chinese documentation index](../zh-CN/INDEX.md); they are not all implemented product features. Source repository: [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go). Release tags and target-platform commitments remain to be selected. Project-owned code and documentation use [Apache-2.0](../../LICENSE); dependencies retain their own licenses. See [contribution guidance](../../CONTRIBUTING.md), [security policy](../../SECURITY.md), [third-party notices](../../THIRD_PARTY_NOTICES.md) and [release preparation](RELEASING.md).

All current guides use `docs/<locale>/` with English directory and file names. Start from the [documentation hub](../README.md).
