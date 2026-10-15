# Preparing a GitHub publication

**English** · [简体中文](../zh-CN/GITHUB.md) · [日本語](../ja-JP/GITHUB.md) · [한국어](../ko-KR/GITHUB.md) · [繁體中文](../zh-TW/GITHUB.md) · [Documentation](../README.md)

## Current decisions

| Item | State |
|---|---|
| GitHub owner | [lorvanto](https://github.com/lorvanto) |
| Repository name and URL | [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go), public |
| Display name | Monitor Go |
| Project license | [Apache-2.0](../../LICENSE) |
| Application/schema baseline | `0.4.0-dev` / schema `8`; not a published release |
| Go module | `github.com/lorvanto/monitor-go` |
| Release targets, tag and date | To be selected after target-specific validation |
| Publishing automation | Not configured; the files in `.github/` are report/review templates |

Creating a public source repository and publishing downloadable releases are separate steps. The source repository is `lorvanto/monitor-go`; downloadable versioned releases remain a separate publication step.

## Source repository checklist

- [x] English landing page and five-language current guides.
- [x] Apache-2.0 text, project NOTICE and direct-dependency license inventory.
- [x] Contribution, support, security, roadmap and Unreleased changelog documents.
- [x] Markdown issue templates for bugs, features and translations; PR and release-note templates.
- [x] Runtime data, backups, credentials, dependencies and build outputs excluded by `.gitignore` for the documented paths.
- [x] Select the public repository `lorvanto/monitor-go`; description and topics describe the current monitoring scope.
- [x] Review the exact staged file list and contents before the initial commit. `.gitignore` does not sanitize already tracked files or data saved outside ignored paths.
- [x] Rebuild and test an isolated export of the staged source without adjacent repositories or runtime data.
- [x] Set the Go module and all internal imports to `github.com/lorvanto/monitor-go`.
- [x] Enable and verify Issues and private vulnerability reporting.
- [ ] Configure review/branch protection and automated checks when needed; these are not currently configured.

Review helpers, run locally:

```sh
git status --short --untracked-files=all
git ls-files
git diff --cached --stat
git diff --cached --check
npm --prefix web run check:docs
```

Review staged contents locally as well. Check source/examples for passwords, tokens, private hostnames, personal machine paths and real screenshots. Use synthetic examples and reserved example domains. Never upload `data/`, `tmp/`, `backups/`, Agent identity files or local audit logs. An empty command result is not a complete secret scan.

The public source repository already exists. For subsequent synchronization, review the changes, verify that the remote is `https://github.com/lorvanto/monitor-go.git`, and push the authorized branch. Repository creation steps apply only when creating a separate repository; avoid adding an unrelated generated README/license to prepared local history. Add version tags, release badges and download links only after the corresponding releases and assets are published and verified. These are publication actions to perform when requested, not commands run by this document.

GitHub recognizes the supplied Markdown templates on the repository's default branch; see its [template documentation](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/about-issue-and-pull-request-templates). Enable [private vulnerability reporting](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository); adding `SECURITY.md` alone does not enable it.

## Release candidate checks

Run from a clean candidate checkout using an isolated data directory:

```sh
make test
npm --prefix web run format:check
make build
```

Also run Go formatting checks on the candidate and scan dependencies with a current, pinned vulnerability scanner and `npm --prefix web audit`. Record the tool versions, date, findings and remediation decisions. A previous scan is not evidence for a new build. npm audit may contact the configured registry; use the project's approved registry.

For each advertised OS/architecture, verify installation, first-admin setup, HTTPS browser access, permissions, target checks, Agent enrollment/reconnect, file upload/download, notifications and clean shutdown on that target. Cross-compilation alone is not runtime validation. Use a controlled notification receiver and synthetic data.

Rehearse upgrade and rollback using copies of test databases for supported schemas. Stop the instance and preserve the entire data directory, including `master.key` and uploads. Document the compatible server/Agent pair and unsupported upgrade paths. Do not test against a live user's database.

## Versioning and packaging

Choose the release version and update `web/package.json`, the root package metadata in `web/package-lock.json`, and the version returned by `internal/app/server.go` / `internal/app/settings.go`. Update `CHANGELOG.md` and affected guides. SQLite schema changes are separate from application versions. The project currently has no `-version` CLI flag or release-time linker version injection; do not document unsupported switches.

Build frontend assets before Go binaries. Name artifacts by product, component, OS and architecture. Each archive should include the appropriate binary, quick-start instructions, `LICENSE`, `NOTICE`, and the complete applicable third-party notices for that artifact. Do not include setup tokens, runtime databases, credentials, test accounts or `node_modules`.

Record SHA-256 checksums for actual archives, the source commit, toolchain, schema and tested platforms. The source README's `make build` creates local binaries; it does not produce release archives, checksums, signatures or container images automatically. A [Dockerfile and Compose workflow](DOCKER.md) build local images from source; no prebuilt registry image or signing service is published.

Create a draft release from the chosen tag and complete the [release-note template](../../.github/RELEASE_TEMPLATE.md). Verify attached files, checksums, license bundle, upgrade instructions and links before publishing. Add badges/download URLs only after they resolve to the actual repository and assets. Retain the prior release and backup instructions for rollback.

## Markdown ownership

The English root documents define the common contribution, security and release process. Localized guides explain the same process; the Apache license text itself remains the official English text. When commands, product scope or policy change, update affected translations in the same change. Historical design and validation records are explicitly labeled and must not be used as present-day release guarantees.

Container changes must pass `docker compose config --quiet` and `node scripts/test-docker.mjs`. See [Docker deployment](DOCKER.md) for the persistent data, host-metric boundaries and local build workflow. Registry publishing requires a separate verified publication task.
