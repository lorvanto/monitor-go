# Project Collaboration Guidelines

These instructions apply throughout this repository. Keep this file and the [Simplified Chinese collaboration guide](docs/zh-CN/COLLABORATION.md) in sync. All file paths and commands below are relative to the repository root.

## Project scope

Use Monitor Go as the project name and `monitor-go` as the repository and Go module identifier. Keep the UI, documentation, protocol identifiers, deployment configuration and examples consistent, and maintain only this project's materials.

This is an independent Go monitoring and operations project with an embedded SQLite file database. It supports multiple collaborators within a single installation, without multitenancy. Implemented capabilities include five core page groups, the Go + SQLite backend, remote Agents, SMTP / Webhook notifications, navigation and file management, image styles, instance settings, operation auditing, application keys, packages, QR codes and barcodes. See the [README](README.md) for current capabilities and boundaries. Continue implementation according to tasks confirmed by the user.

Read the [Chinese documentation index](docs/zh-CN/INDEX.md) first. Documentation distinguishes confirmed requirements, proposed designs and unimplemented features. Never describe a proposal as an implemented fact.

## Development constraints

- Keep all new code, assets, documentation and build configuration within this repository. Do not use neighboring projects as runtime dependencies.
- Do not copy external database configuration, passwords, tokens, private keys, runtime data or Git history.
- Unless requested by the user, do not delete other projects, migrate real databases, create remote repositories or publish releases.
- Organize code by business module, use explicit interface types and let business operations define transaction boundaries.
- SQLite is the only supported storage backend. Do not introduce a MySQL storage adapter, external Redis or a message queue as an installation dependency. MySQL and Redis checks are monitoring capabilities.
- Implement menu visibility separately from server-side authorization. Every new write endpoint must have the corresponding permission check.
- Resources are shared across the installation. Creator fields are for auditing and must not become implicit data isolation filters.
- Prefer soft deletion for business configuration. Expire historical metrics according to an explicit retention policy.
- Preserve Go, frontend and database lockfiles. Record each selected dependency's version and purpose.
- Keep Simplified Chinese documentation in `docs/zh-CN/` and Traditional Chinese documentation in `docs/zh-TW/`. Use English directory and file names, and update the indexes when adding documentation.

## Verification requirements

- After Go implementation changes, check formatting and run `go test ./...`. Run builds, race checks or integration tests as appropriate for the change.
- The frontend uses React 19 + TypeScript + Vite, with Zustand for shared and page state and pages loaded on demand. Use TSX for new components; do not introduce another UI runtime.
- After frontend implementation changes, run type checking, React interaction tests and a production build. Verify actual browser behavior when changing critical interactions.
- Verify database changes against actual SQLite files, including migrations, constraints, transactions and core business integration tests. In-memory substitutes alone are insufficient.
- For API changes, start this project's service and verify real HTTP requests. Record the actual port to avoid testing an old service instance.
- For container changes, run `docker compose config --quiet` and `node scripts/test-docker.mjs`. Verify supported target platforms, persistent data and cold backup/restore with disposable volumes; never mount a live installation's data into a test container.
- For documentation or directory-only changes, perform only the relevant file, link and repository independence checks.

## Internationalization maintenance

- Keep shared messages in `locales/`. The initial languages are Simplified Chinese, English, Japanese, Korean and Traditional Chinese. Update all five catalogs for new visible messages and preserve interpolation parameters. New browsers start in English; restore a valid saved choice instead of automatically using browser language preferences.
- Never use display text as status codes, permission keys, query values or database enums. Preserve user-authored names, notes, files and historical records in their original language.
- For internationalization changes, run `npm --prefix web run check:i18n` and `npm --prefix web test`, and verify that switching languages preserves unsaved edits.
- The root README is the English entry point. Organize all five documentation languages under `docs/<locale>/`, with Simplified Chinese design documents in `docs/zh-CN/`. Update all five current guides when deployment commands, supported scope or prerequisites change.

## GitHub documentation and publishing

- Project-owned code and documentation use Apache-2.0. Preserve `LICENSE`, `NOTICE` and third-party license texts. Update `THIRD_PARTY_NOTICES.md` when adding dependencies.
- Use English for root community documents. Keep Chinese publishing and contribution guidance in [docs/zh-CN/GITHUB.md](docs/zh-CN/GITHUB.md) and maintain the related entry points in all five languages.
- The Unreleased section in `CHANGELOG.md` and the application version do not establish that a release has been published. Do not invent tags, download URLs, CI status or platform validation results.
- For documentation changes, run `npm --prefix web run check:docs`. It covers the root directory, `docs/` and `.github/`. When changing the checker itself, also run `npm --prefix web run test:docs`.
- Create remote repositories, push changes and publish releases only when actually assigned a publishing task. Preparing local documentation does not mean publication has occurred.
