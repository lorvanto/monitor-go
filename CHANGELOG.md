# Changelog

User-visible changes belong here. Entries under **Unreleased** describe the current development tree; they are not published releases. Application version `0.4.0` and schema version `4` currently identify the implementation baseline. A maintainer will create a dated release entry only when an actual tag and release are published.

## Unreleased

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

### Upgrade considerations

- SQLite migrations support schema versions 1 → 2 → 3 → 4. Stop the service and back up the entire data directory before upgrading. Older binaries cannot read schema 4 directly; rollback requires the complete pre-upgrade backup.
- The React client and locale changes require no new database migration. Rebuild and restart the Go binary to serve the updated assets. Browser API and upload calls carry `Accept-Language`; status codes and machine-readable fields remain stable. Missing language headers preserve the original Chinese API behavior.
- Built-in navigation may include an optional `nameKey` for presentation. Custom names remain literal. Permanent file deletion uses the fixed `DELETE` wire token while accepting the legacy Chinese confirmation value.

### Known limitations

See the [current user guide](docs/en-US/README.md) and [roadmap](ROADMAP.md). Public release assets, prebuilt registry images, target-host capacity validation and automatic publishing are not established by the source-built Docker support.
