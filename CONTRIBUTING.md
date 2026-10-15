# Contributing to Monitor Go

[简体中文](docs/zh-CN/GITHUB.md) · [日本語](docs/ja-JP/GITHUB.md) · [한국어](docs/ko-KR/GITHUB.md) · [繁體中文](docs/zh-TW/GITHUB.md) · [Documentation](docs/README.md)

Bug reports, focused fixes, documentation improvements and translation corrections are welcome. Discuss substantial behavior or architecture changes in an issue before implementation. Use the repository's issue and pull request templates in [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go). Report vulnerabilities through [SECURITY.md](SECURITY.md), rather than public bug reports.

## Development environment

Use Go 1.26.9 or a compatible newer version, Node.js 22.12+ / 24.x, npm and Make. Run commands from the repository root. Go and npm dependencies are pinned in `go.mod`, `go.sum` and `web/package-lock.json`. The documented shell commands target a POSIX shell; Windows contributors can use WSL.

```sh
make build
./bin/monitor -data tmp/development -listen 127.0.0.1:5093
```

Read `tmp/development/setup-token` locally to initialize a development account. In a second terminal:

```sh
MONITOR_GO_API_TARGET=http://127.0.0.1:5093 npm --prefix web run dev
```

Open the frontend at `http://127.0.0.1:5188`. Build `web/dist` before compiling or testing Go: the server embeds those assets. Go changes require rebuilding and restarting the server; frontend development uses hot reload. Do not use a production data directory for development.

The frontend uses React 19, TypeScript, Vite and Zustand. Add business pages in `web/src/views/`, shared components in `components/`, shared application state in `stores/`, and pure helpers/types in `lib/`. Keep form state stable during locale changes. Components subscribe through hooks; do not mutate shared stores during rendering.

## Implementation rules

Read the repository-wide [collaboration guidelines](AGENTS.md), also available in [Simplified Chinese](docs/zh-CN/COLLABORATION.md).

- Keep the repository independently buildable. Do not depend on adjacent checkouts or copy their credentials, databases or Git history.
- SQLite is the only application datastore. MySQL and Redis are monitored services; do not add them as installation dependencies.
- Organize changes by business operation with explicit types and transaction boundaries. Shared resources belong to the installation; creator fields are audit metadata.
- Check permissions in every write API. Hiding a menu is not authorization. Use version checks for concurrent edits and soft deletion for business configuration where applicable.
- Keep network I/O outside long database transactions. Validate database behavior using actual temporary SQLite files.
- Preserve lockfiles. Document the version, purpose and license of a new dependency in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
- Do not commit databases, uploads, logs, credentials, enrollment tokens, private keys, backups or local screenshots containing private information.

## Verification

```sh
make test
npm --prefix web run format:check
```

`make test` installs the locked frontend dependencies, checks locale catalogs and documentation links, typechecks and builds the frontend, runs frontend tests, then runs Go race tests and vet. Use `gofmt` on changed Go files. For a focused Go change, also run `go test ./...`; for an API change, start an isolated instance and exercise the actual HTTP endpoint, recording the port and result in the PR.

For frontend interaction changes, verify the browser at desktop and narrow widths, including empty/error states and the relevant roles. For localization changes, check all five languages, preference persistence and preservation of unsaved input. For documentation-only changes:

```sh
npm --prefix web run check:docs
npm --prefix web run test:docs
```

The documentation check needs Node.js only; it does not require `npm ci`. Do not report a check as passed if it was skipped. Describe the reason and remaining validation in the PR.

## Documentation and translations

For container changes, run `docker compose config --quiet` and `node scripts/test-docker.mjs`. Docker Engine / Docker Desktop with Linux containers, BuildKit, Compose v2.20+ and Node.js 22.12+ / 24.x are needed for this test. It builds the image and verifies a disposable installation, uploads, recreation and cold backup/restore on an automatic loopback port. It removes only its own test containers and volumes. Follow the [Docker guide](docs/en-US/DOCKER.md) and keep all five Docker guides aligned.

The root README is the English landing page. All five languages use `docs/<locale>/`, including `docs/zh-CN/` for Simplified Chinese and `docs/zh-TW/` for Traditional Chinese. Use English directory and file names while keeping document content in its intended language. Keep deployment commands and capability boundaries aligned across the five current guides. Historical plans do not establish shipped behavior.

The [README screenshot gallery](README.md#screenshots) uses repository-local JPEG files in `docs/assets/screenshots/`. Refresh captures from the running application with English selected and the light theme. Use a separate temporary data directory with synthetic English names, documentation-only addresses and illustrative metrics; keep local host collection disabled with `-collect-host=false`. Never capture real infrastructure, user files, credentials, tokens or private logs. Wait for the page to finish loading, preserve the actual interface, check each image visually and keep the gallery captions aligned with implemented features. These images illustrate the interface; their values are not benchmark or production-validation evidence.

UI and API messages share `locales/*.json`. Preserve message keys, interpolation parameters, permission keys, API fields and stored values. Never translate user-authored names or records. Follow the [translation guide](docs/en-US/INTERNATIONALIZATION.md) and update [the documentation index](docs/README.md), including the Chinese index for new Chinese documents.

## Pull requests

Describe the concrete problem, final behavior, verification performed and any compatibility or migration impact. Keep unrelated refactors out of a bug fix. Include screenshots for visible changes with synthetic data and update [CHANGELOG.md](CHANGELOG.md) under Unreleased when behavior changes. Changes to dates or version labels alone do not constitute a release.

Be respectful and discuss code and evidence rather than individuals. Keep reports reproducible and redact private information. Maintainers may ask for a smaller scope or additional evidence; no response-time commitment is made.

Unless explicitly stated otherwise, contributions intentionally submitted for inclusion are under [Apache-2.0](LICENSE), as described in its contribution terms. Submit only material you have authority to contribute and retain third-party attribution. No separate CLA or DCO workflow is configured.
