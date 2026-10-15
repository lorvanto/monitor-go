# Changelog

User-visible changes belong here. Entries under **Unreleased** describe the current development tree; they are not published releases. Application version `0.4.0-dev` and schema version `8` currently identify the implementation baseline. A maintainer will create a dated release entry only when an actual tag and release are published.

## Unreleased

### Five-language phone layouts

- Keep time-range buttons, long application/host/alert names and file/package selectors within narrow layouts. Bound dialog titles and move long context text into the scrolling body.
- Keep dialog close/footer actions visible, enlarge mobile footer tap areas and wrap long action labels. Follow the visual viewport and reveal the focused field during keyboard resizing without losing drafts.
- Add five-language viewport/focus regressions and browser checks across portrait, landscape and reduced-height layouts. No dependency or schema change.

### Japanese, Korean and Traditional Chinese audit

- Standardize Japanese check/administrator terms, Korean Agent enrollment wording and Traditional Chinese sign-in, keys, archive and network terms across catalogs and current guides.
- Add an in-dialog language selector; preserve unsaved fields, selections and typed deletion confirmations while switching languages, without submitting or reloading forms.
- Translate malformed API/package-upload errors and make unknown-route errors reactive. Add mixed-language catalog guards and page, dialog, draft and five-language HTTP regressions. No dependency or schema change.

### English UI and form validation

- Localize native field-validation bubbles, stored errors and transient notices across setup, sign-in and dialogs, including in-flight responses and language switches without resubmission. Preserve typed drafts and translate untouched default editor names.
- Carry the UI language on file/package downloads and image requests. API errors retain their readable message and add translation keys and arguments.
- Keep new empty check groups language-neutral and distinguish default-group filtering from all groups. Preserve existing names and historical data; no schema or dependency change.
- Complete 24 missing fixed audit-action translations without rewriting stored history. Add dialog/selector, HTTP, SQLite and language-switch regressions; reject Chinese catalog values in English and enforce shared form validation.

### Bounded read concurrency

- Keep one writer and route reporting queries through two read-only SQLite connections, protected by URI mode and per-connection query-only pragmas. Preserve transactional reads, authorization and scheduling paths, migrations, seed and backup behavior.
- Add a reproducible concurrent polling/write benchmark and regression coverage for read-only enforcement, WAL snapshots, rollback, connection replacement and shutdown. No schema or dependency change.

### Hourly history and long-term availability

- Schema 8 adds transactional hourly rollups and pending-row tracking, including idempotent reruns and late results. Raw host/check retention defaults to 7 days (1–30 configurable), check summaries to 90 days (90–730 configurable). Roll up before cleanup; no host summary table is added.
- Add permission-checked 30/90-day availability and overview ranges, weighted by actual checks, with hourly UTC boundaries and explicit no-data values. Existing deleted history is not reconstructed.
- Stop the service and back up the complete data directory before upgrading; older binaries cannot read schema 8.


### Agent and collector regression coverage

- Add deterministic Agent enrollment, persisted credential, report, retry, redirect and private-CA tests using local HTTP/TLS fixtures. Preserve terminal authentication failures and capped linear backoff for retryable failures.
- Exercise host collection with injected dependencies, partition limits and partial failures. Avoid invalid network rates when the sample clock does not advance or counters reset.


### HTTP response rules

- Use response-time incident titles for HTTP latency warnings; retain certificate titles for SSL warnings.
- Schema 7 adds bounded response keyword matching, inverse matching and latency warnings to HTTP check creation/editing. Connection/status and keyword failures take precedence; details flag the 1 MiB inspection limit without exposing response bodies.
- Stop the service and back up the full data directory before upgrading. Older binaries cannot read schema 8. Optional custom headers and method selection are not implemented.


### Default resource localization

- Localize built-in labels and selectors inside menu management as well as navigation. Keep default name translations when only changing order/visibility, and preserve explicit name drafts across language changes.

- Localize untouched seeded folders and full selector paths, roles/descriptions, image styles and local-host labels in all five languages. Translate fixed audit action labels at display time. Preserve custom names and stored historical data; do not infer translation from arbitrary display text.


### Online backup

- Add a read-only `monitor backup` command that snapshots SQLite without the service lock or migrations, verifies each referenced object, and records hashes and versions. Failed backups retain an incomplete marker and cannot be opened as an installation. Restore only while stopped.


### Build metadata and continuous integration

- Share one injectable build version across server status, system details, Agent reports and all outbound User-Agent headers. Add `-version` to both binaries and version build arguments for Make/Docker/Compose.
- Add a push/PR workflow with pinned official Action commits, Go from go.mod and Node 22, running tests, builds and formatting checks. Remote execution is verified separately from this configuration.


### Static asset caching

- Precompute strong ETags and gzip content at startup. Hash-named assets receive one-year immutable caching; HTML/unhashed files revalidate. Preserve HEAD, ranges, conditional requests and security headers without new dependencies.


### Page refresh and batched lists

- Poll only the active page’s required data, session, navigation and a lightweight mobile alert badge. Load team metadata on entry or mutation; preserve session and page race protection.
- Batch monitor availability/history into three SQL queries and alert activity into two, preserving response contents and limits. Indexed history candidates bound window-function work to the latest 32 results per monitor.


### Configurable host thresholds

- Schema 6 adds global CPU, memory, disk and sustained-duration settings. Local and remote hosts share continuous-window validation, recovery and disable rules. Settings retain authorization, optimistic locking and audit records.
- Stop the service and back up the complete data directory before upgrading; older binaries cannot read schema 8.


### Available in the initial baseline

- Go server with embedded React interface, SQLite storage, guarded first-admin setup, sessions, CSRF, members and role permissions.
- Local/remote host metrics, HTTP/TCP/SSL/MySQL/Redis service checks, incident acknowledgement/silence/recovery, SMTP and signed Webhook delivery with persistent retries.
- Custom navigation, shared private files, image styles, trash/restore, audit history and instance settings.
- Scoped application keys, package distribution, QR codes and four barcode formats.
- Simplified Chinese, English, Japanese, Korean and Traditional Chinese UI catalogs, API validation messages and current guides. Locale changes preserve unsaved forms and stored user data.
- Shared types and formatters, catalog/placeholder checks, documentation link checks and consistent frontend formatting.
- All five documentation languages organized under `docs/<locale>/` with English directory and file names, updated indexes and cross-language links.
- Align translated guide navigation, release version sources and the permanent-deletion example; normalize copied license filenames. Documentation checks now verify path case, heading anchors, English naming and five-language navigation, with focused regression tests.
- Apache-2.0 project license, contribution/support/security guidance, GitHub issue/PR templates and a maintainer release checklist.

### Frontend modernization

- React 19 and TypeScript client with Zustand state, Vite builds and lazy-loaded business pages. Existing API, permissions, five languages, themes and embedded Go deployment are preserved.
- React Testing Library and Vitest cover rendering, forms, confirmations, navigation, focus, charts and session expiry. Type checking includes tests.

### React follow-up fixes

- Discard responses from earlier sessions, clear account-specific UI state on logout, and wait for a fresh read after writes overlap polling. Network failures during logout return to the login screen with an error message.
- Keep host charts scoped to the selected host and time range. Bind host-note edits and incident-silence dialogs to the original resource and version.
- Preserve the latest file preview and application-key selection when requests finish out of order. Recover file pagination when the final page becomes empty; ignore closed release-editor requests.
- Apply cleared audit filters during an active request, and prevent failed queries from appending to older results. Keep theme switching usable when browser storage is unavailable. Add a translated recovery screen for failed lazy page loads.
- Add 16 focused React regression tests covering these cases. See the [React review record](docs/zh-CN/REACT_MIGRATION.md) for verification scope and limitations.

### Default interface language

- First visits to setup and sign-in start in English, regardless of browser language. Valid saved language choices are restored. Invalid preferences or unavailable browser storage also use English; switching languages preserves setup form input.

### Docker deployment

- Add a multi-stage Docker build with pinned base digests and a non-root runtime, plus Compose with a persistent data volume, read-only root, health check and explicit HTTPS settings. Five-language guides and a disposable container smoke test cover initialization, recreation and cold backup/restore. Container-local host collection is off by default; native Agents monitor real hosts. Images are built locally from source.

### Scheduler queue performance

- Add the schema 5 partial `jobs_queue` index for the FIFO queued-job lookup, so completed job history does not require a full table scan on each scheduler tick. Scheduling behavior and the seven-day job retention policy are unchanged. File-backed migration tests cover data preservation, indexed query plans, queue order, restart and transactional rollback.

### Trusted proxies and login limits

- Count only failed logins by client IP (30/5 minutes) and normalized username (10/5 minutes). Successful logins do not consume or reset the budgets. Setup counts all attempts separately.
- Read forwarded addresses only through explicitly configured `-trusted-proxy` CIDRs (`MONITOR_TRUSTED_PROXY` in Compose); invalid CIDRs prevent startup. Full limiter tables reject new keys while preserving existing clients.

### Monitor editing and archiving

- Add permission-checked full monitor updates, explicit credential retention/replacement/removal, optimistic conflicts, audit records and queued-job cancellation. Preserve history and existing incidents.
- Add translated React edit forms and archive confirmation; conflicts preserve unsaved input while refreshing server data.

### Default instance-name localization

- Translate the untouched default instance name with the browser language instead of displaying the stored Chinese seed in English pages. Add an explicit default-name option in settings; preserve custom names, including Chinese names. No schema migration is required.

### Chat robot Webhooks

- Add DingTalk Markdown, Feishu text and WeCom Markdown formats without a database migration. Preserve generic JSON Webhooks and their signature headers.
- Implement platform signatures, strict business-result checks and persisted retries, with bounded chat text and original stored alerts retained. Add five-language format selection and configuration guidance. Validation uses local HTTP fixtures; real robot accounts were not tested.

### Upgrade considerations

- SQLite migrations support schema versions 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. Stop the service and back up the entire data directory before upgrading. Older binaries cannot read schema 8 directly; rollback requires the complete pre-upgrade backup.
- The React client and locale changes require no new database migration. Rebuild and restart the Go binary to serve the updated assets. Browser API and upload calls carry `Accept-Language`; status codes and machine-readable fields remain stable. Missing language headers preserve the original Chinese API behavior.
- Built-in navigation may include an optional `nameKey` for presentation. Custom names remain literal. Permanent file deletion uses the fixed `DELETE` wire token while accepting the legacy Chinese confirmation value.

### Known limitations

See the [current user guide](docs/en-US/README.md) and [roadmap](ROADMAP.md). Public release assets, prebuilt registry images, target-host capacity validation and automatic publishing are not established by the source-built Docker support.
