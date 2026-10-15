# Monitor Go — user guide

**English** · [简体中文](../zh-CN/README.md) · [日本語](../ja-JP/README.md) · [한국어](../ko-KR/README.md) · [繁體中文](../zh-TW/README.md) · [Documentation](../README.md)

> Status: pre-release development baseline `0.4.0-dev`, SQLite schema `8`; no published release is implied. Examples use a POSIX shell; Windows developers can use WSL.

Schema 5 adds a partial index for queued scheduler jobs, avoiding scans of completed-job history. Job retention remains seven days. Stop the service and back up the complete data directory before upgrading; older binaries cannot read schema 8, and rollback requires that backup.

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

Configure SMTP, generic Webhook or chat-robot channels in the alert center. SMTP requires TLS or STARTTLS. Webhooks require HTTPS, except numeric loopback for testing. Notifications use persistent jobs with up to four attempts. Delivery is at least once; receivers should deduplicate by `X-MonitorGo-Delivery-ID`. SMTP acceptance does not guarantee inbox delivery. Silence skips pending notifications without later catch-up.

Generic Webhook signatures use `X-MonitorGo-Signature: sha256=<hex HMAC-SHA256>`. Sign the timestamp header, a period, and the original body bytes with the channel secret. Validate signatures and a timestamp window at the receiver.

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

Database versions 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 migrate transactionally. Older binaries cannot directly read schema 8; rollback requires the entire pre-upgrade backup. New permissions are not silently assigned to custom roles. This internationalization change requires no new schema migration.

## Current limits

There is no multitenancy, external storage adapter, remote command execution, agent auto-update, online backup UI, public file sharing. Certificate issuance remains planned. Configured ceilings of 500 checks and 1000 agents are not load-test claims. Production Linux sampling, real email-provider delivery and capacity testing require deployment-specific validation.

Historical design proposals and detailed validation records remain in the [Chinese documentation index](../zh-CN/INDEX.md); they are not all implemented product features. Source repository: [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go). Release tags and target-platform commitments remain to be selected. Project-owned code and documentation use [Apache-2.0](../../LICENSE); dependencies retain their own licenses. See [contribution guidance](../../CONTRIBUTING.md), [security policy](../../SECURITY.md), [third-party notices](../../THIRD_PARTY_NOTICES.md) and [release preparation](RELEASING.md).

All current guides use `docs/<locale>/` with English directory and file names. Start from the [documentation hub](../README.md).

## Trusted proxies and login limits

`-trusted-proxy` accepts comma-separated proxy CIDRs and defaults to empty (no trusted proxies). Invalid CIDRs prevent startup. Trust only the actual proxy addresses as seen by this process; the proxy must overwrite or safely append `X-Forwarded-For`. The server walks that header from right to left, stopping at the first untrusted address. Headers from other peers are ignored. Login counts only failures: 30 per client IP and 10 per normalized username per five minutes; successful logins neither consume nor reset those budgets. Setup counts every attempt (10 per IP per five minutes).

## Monitor editing and archiving

Use Edit on a monitor to change its complete configuration. Blank passwords keep saved credentials; a new password replaces them, and Clear saved credentials removes them. HTTP/TCP/SSL clear database credentials. Updates use optimistic versions and reset the current state and schedule, cancelling queued jobs. Existing history and unresolved alerts are retained. Archive requires confirmation and stops future checks. Read-only members have neither action. A 409 conflict refreshes server data while retaining your form; reopen the editor to review the latest configuration before saving again.

The default instance name follows the interface language (English: “My monitoring center”). In System settings, select “Use the default instance name” to restore that behavior. Custom names remain exactly as entered in every language. Untouched legacy defaults are recognized automatically; previously saved names are preserved until you explicitly choose the default.

## Chat robot Webhooks

Webhook message formats are Generic, DingTalk, Feishu and WeCom; existing channels remain Generic. Select the format in the channel editor. DingTalk uses Markdown with optional URL timestamp/signature (milliseconds); Feishu uses text with optional body timestamp/signature (seconds). WeCom uses Markdown and the `key` in its URL, with no separate signing secret. Changing format in the editor clears the previous signing secret.

Generic keeps its existing JSON and signature-header contract. Chat formats require a successful HTTP response **and** platform code 0 (`errcode` for DingTalk/WeCom, `code` for Feishu); malformed or missing codes fail. Failures retry after 15 seconds, 1 minute and 5 minutes. Messages include event, original title/resource/description, severity, UTC time and delivery ID. Fixed labels are English. Long fields are truncated in chat messages to fit WeCom’s 4096-byte Markdown limit; stored alerts remain unchanged. No real-platform delivery was verified: local HTTP fixtures cover formats, signing, errors and retries. Use Test on your own bot to verify credentials and platform policies.

[DingTalk](https://open.dingtalk.com/document/robots/customize-robot-security-settings) · [Feishu](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot) · [WeCom](https://developer.work.weixin.qq.com/document/path/91770)

## Host alert thresholds

System settings provides installation-wide CPU, memory and disk thresholds (defaults: 90%, 90%, 85%) and a sustained duration of 1–60 minutes (default: 5). Set an individual threshold to 0 to disable that metric alert. Schema 6 adds these validated settings; stop the service and back up the whole data directory before upgrading. Older binaries cannot read schema 8.

Local and remote hosts use the same rules. CPU/memory must stay at or above the threshold throughout the duration: samples must cover the whole window, including a sample at or before its start, with no missing values or gaps over 45 seconds. Insufficient data does not open an alert or resolve an existing one. The latest valid reading below the threshold resolves it. Disk alerts use the latest available disk readings immediately. Disabling a threshold resolves that metric's incident on the next report; settings changes also take effect on the next report. The 90-second remote-host offline rule is unchanged. Per-host overrides are not implemented.

## Page refresh and list queries

The 10-second visible-page refresh loads hosts/checks/incidents only where needed, plus the current session and navigation. Other pages use a small alert-count endpoint for the mobile badge, with the same latest-200-event scope as the incident list. Team roles, members and permissions load on entry to their page or after a change, not on every poll. Page switches and writes trigger a fresh load while stale-page and prior-session responses are discarded. Monitor lists use three SQL queries and incident lists use two; the response shape, 24-hour availability, latest 32 check results and latest 100 activities per incident are preserved.

## Static asset caching

The server prepares strong ETags and gzip representations at startup using the Go standard library. Hashed files under `/assets/` use `public, max-age=31536000, immutable`; HTML and other unhashed files use `no-cache` and conditional validation. Gzip negotiation respects exclusions and quality preferences, with `Vary: Accept-Encoding` and distinct validators for compressed/uncompressed bytes. Conditional GET/HEAD returns 304 when appropriate; byte ranges and existing security headers remain supported. No external compression service is required.

## Build versions and CI

Server, Agent reports and outbound User-Agent headers share `internal/buildinfo.Version`. Direct `go build` uses `0.4.0-dev`; `make build` injects `git describe --tags --always --dirty`, falling back to that default without Git metadata. Use `make build VERSION=0.4.0-dev` to select a value explicitly. Both binaries support `-version`. The frontend package uses the development baseline; the System settings page reports the actual server build. GitHub CI is configured for push and pull requests, using Go from `go.mod`, Node 22, pinned Action commits, race tests, type checks, build and formatting checks. A workflow file does not establish a passing remote run or a published release.

`make docker-build` injects the Git-derived version. With plain Compose, supply `MONITOR_VERSION` explicitly; without it, the image reports `0.4.0-dev`. For direct builds use `docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || printf 0.4.0-dev)" -t monitor-go:local .`. Git history stays outside the Docker build context.

## Online backup and restore

The service may keep running during this command. The output directory must not exist and its parent must exist. The command opens the source read-only without acquiring the instance lock or migrating it (supported schemas: 3 through the current version). It creates a consistent SQLite snapshot and copies the master key and all objects referenced by that snapshot, including trash. New uploads after the snapshot are excluded. Missing objects are retried; missing, corrupt or changed objects fail with a nonzero exit status and retain BACKUP_INCOMPLETE. Never restore such a directory. The completion manifest records UTC time, build/schema versions, object count and SHA-256 hashes. Setup tokens, locks, staging and image caches are excluded.

```sh
mkdir -p backups
./bin/monitor backup -data data -out backups/snapshot-001
```

To restore, stop the service, replace the entire data directory with a complete backup, and start a binary supporting at least its schema version. Keep the old directory until verification succeeds. Restore offline only; do not merge files into a running installation. Treat backups as private credentials and copy them to independent storage. Cold backups remain supported.

## Default resource labels

Untouched seeded folder names (including paths in settings and package selection), role names/descriptions, image style names and the local-host alias follow the selected interface language. Renamed values remain literal; the server supplies explicit translation keys so a custom name matching a default is not translated by its text alone. Stored historical records remain unchanged.

Fixed audit action labels are localized at display time; actor names, resource text and original stored audit rows are preserved.

Menu management also localizes built-in names, page destinations, parent groups and permission options. Changing visibility or order without editing the name preserves its default translation behavior; an explicitly edited name remains user-authored text and survives language switching.

## HTTP response rules

Create and edit HTTP checks with an optional UTF-8 response keyword (up to 1024 bytes), inverse matching and a response-time warning threshold (0–30000 ms; 0 disables it). Keyword matching is case-sensitive and examines only the first 1 MiB of the decoded body; details explicitly report truncation. Blank keywords preserve status-only checks. Latency includes reading the inspected body. Connection/status failures take priority, then keyword failures, then latency warnings. Response content is never copied into details. Non-HTTP checks clear these rules. Custom headers and GET/HEAD selection remain optional future work. Schema 7 adds the fields; stop and back up before upgrading, as older binaries cannot read it.

HTTP latency incidents use a response-time title; SSL certificate warnings keep their certificate-specific title.

## Agent retry and collection verification

Network/server failures use a linear delay of 5 seconds per consecutive failure, capped at 60 seconds; a successful report restores the normal 15-second interval. HTTP 401/403, and 409 during enrollment, stop the Agent because credentials are invalid or enrollment conflicts. Re-enroll with a new configuration instead of endlessly retrying revoked access. Automated tests use local HTTP/TLS servers and injected collectors/clocks, covering credential persistence, reports, retry cap/reset, redirects, private CAs, partition filtering (up to 16 usable unique mounts), partial metric errors and network counter/time resets.

## Long-term availability and retention

Schema 8 adds hourly check summaries (total/good counts and average/maximum latency), transactional consumed-row markers and an index for pending rows. At startup and each hourly maintenance run, closed hours are accumulated before raw data is removed. Repeated runs and late results do not double-count; a failed rollup preserves raw rows. Settings default to 7 days of raw host/check metrics (1–30 configurable) and 90 days of check summaries (90–730 configurable). Cleanup is batched and aligned to UTC hours, so raw retention can exceed the requested duration by an hour plus the maintenance backlog. There is no host summary table. Stop and back up before upgrading; older binaries cannot read schema 8.

Overview and Service checks offer 30/90-day availability. GET /api/v1/monitors/availability?hours=720 or 2160 requires monitor:read. Overview accepts the same ranges. Results combine hourly summaries and unconsumed raw rows in one SQLite snapshot, weighted by actual check count. Healthy and warning checks count as available, matching the existing 24-hour definition. Windows start at a UTC hour boundary; the current partial hour is included. Missing samples are not inferred as successes, so no history yields null/“—”. Previously deleted history cannot be reconstructed.

For capacity estimates, raw check rows ≈ targets × (86400 / intervalSeconds) × rawDays; summary rows ≤ targets × 24 × summaryDays. Host raw rows ≈ hosts × 5760 × rawDays. Measure bytes per row including indexes on a disposable database with representative details; use (page_count − freelist_count) × page_size to compare used-page growth. Add WAL headroom, uploads and backup copies separately. Deletion makes pages reusable and does not necessarily shrink the file; these formulas are estimates, not capacity guarantees.

## SQLite read concurrency

Reporting queries for monitors, availability, overview, alerts and host metrics use a bounded pool of two read-only connections (`mode=ro`, `query_only=1`) alongside the existing single writer. Authorization, scheduling, mutations, migrations and seed retain the writer path; online backup keeps its separate snapshot connection. Transaction callbacks read through their own transaction. WAL, FULL synchronization and the instance lock remain in place; this adds no schema migration or dependency.

On an Apple M1 / Go 1.26.9 / real WAL file, 500 targets × 1,440 results, four concurrent pollers (five reads each) and 80 paced writes gave median write P95 of 866.6 ms with one shared connection and 0.478 ms with two readers. Median workload time fell from 10.45 s to 5.713 s. These are synthetic local measurements, not a production capacity guarantee. Reproduce with `go test ./internal/app -run '^$' -bench BenchmarkConcurrentPollingWrites -benchtime=1x -count=3`; the benchmark compares shared, two-reader and four-reader modes and excludes fixture creation from timing.

## Form and error language

Setup, sign-in, dialogs, browser field-validation messages and transient notices follow the selected interface language, including after switching languages without submitting again. Untouched default names in open editors also follow the language; typed names and other unsaved input are preserved. New checks with an empty group display a localized default group. Existing group names and historical records remain literal.

## Language switching inside dialogs

Every shared dialog includes a language selector, so you can switch without closing the editor. Typed names, notes, passwords, checkbox and dropdown selections remain intact. A deletion confirmation remains valid in the language used when it was typed; the server still receives the fixed `DELETE` token. Malformed API/upload responses use translated errors, and unknown-route errors carry the same translation key as other API failures.

## Phone layouts and dialogs

On narrow screens, time-range controls wrap and long application, host and alert names break within their panels. Dialog content scrolls independently while close and footer actions remain available. Long context text scrolls with the body; long titles have a bounded scroll area. File/image preview selectors and package-directory controls fit the available width.
