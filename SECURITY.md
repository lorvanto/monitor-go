# Security policy

## Supported versions

The current development baseline reports application version `0.4.0` and SQLite schema version `6`. It is a pre-release development state, not evidence of a published GitHub release. Security fixes currently target the current development code. No older release branch, support period or response-time SLA is promised.

## Reporting a vulnerability

Private vulnerability reporting is enabled for this repository. Use [Report a vulnerability](https://github.com/lorvanto/monitor-go/security/advisories/new), or open **Security → Advisories → Report a vulnerability**. See [GitHub's private-reporting instructions](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/report-privately).

If that button is unavailable, open an issue asking for a private security contact, without exploit details, credentials, target addresses or sensitive logs. Do not post a vulnerability as a public bug report. This repository does not currently publish a separate security email address.

A private report should include the affected commit/version, operating system, deployment mode, prerequisites, minimal reproduction using synthetic data, expected/actual behavior, impact and any suggested fix. Never attach a production database, `master.key`, session cookie, API key or Agent configuration. Coordinate disclosure with the maintainer before publishing details.

## Deployment boundaries

- Use trusted HTTPS for remote browser and Agent access. Set the exact `-public-url` origin and `-secure-cookie` for HTTPS browser deployments.
- Docker Compose binds to loopback and runs a non-root service with a read-only root filesystem and a dedicated data volume. Set `MONITOR_PUBLIC_URL` and `MONITOR_SECURE_COOKIE=true` behind a trusted HTTPS proxy. Use native Agents for actual host metrics; the container does not receive a Docker socket or host filesystem access. See [Docker deployment](docs/en-US/DOCKER.md).
- Monitor targets may be private or loopback addresses by design. Grant target-management permission only to trusted operators; network isolation remains the deployer's responsibility.
- One installation shares resources across its members. It is not a multi-tenant isolation boundary, and menu visibility is separate from server authorization.
- Protect the complete data directory and backups. Database encryption of stored secrets depends on the separate `master.key`; file uploads and database records are not a whole-disk encryption feature.
- Back up after a clean stop, including database companion files, `master.key` and original uploads. Restore into an isolated directory before upgrading a production installation.
- Give application keys the minimum necessary scopes and expiry. Never forward a Bearer key to an external package download URL.

Historical scan results are point-in-time evidence. Re-run dependency and reachable-vulnerability checks for each release candidate; no zero-vulnerability claim is made for future builds.

Localized contribution and security guidance: [简体中文](docs/zh-CN/GITHUB.md) · [日本語](docs/ja-JP/GITHUB.md) · [한국어](docs/ko-KR/GITHUB.md) · [繁體中文](docs/zh-TW/GITHUB.md).
