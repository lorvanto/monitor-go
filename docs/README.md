# Documentation

Start with a current guide in your preferred language. The application is a pre-release development baseline (`0.4.0`, SQLite schema `4`). The project uses Apache-2.0; third-party components retain their own licenses.

All documentation uses English directory and file names under `docs/<locale>/`: `en-US`, `zh-CN`, `ja-JP`, `ko-KR` and `zh-TW`. Each directory contains a current `README.md` guide; Simplified Chinese design documents and historical records live in `zh-CN/` with an `INDEX.md` entry point.

## Current guides

| Language | User guide | Translation maintenance | GitHub and contribution guidance |
|---|---|---|---|
| English | [User guide](en-US/README.md) | [Internationalization](en-US/INTERNATIONALIZATION.md) | [Release preparation](en-US/RELEASING.md) · [Contributing](../CONTRIBUTING.md) |
| 简体中文 | [完整指南](zh-CN/README.md) | [国际化维护](zh-CN/INTERNATIONALIZATION.md) | [GitHub 发布与协作](zh-CN/GITHUB.md) |
| 日本語 | [利用ガイド](ja-JP/README.md) | [翻訳の保守](ja-JP/INTERNATIONALIZATION.md) | [GitHub公開と貢献](ja-JP/GITHUB.md) |
| 한국어 | [사용 안내](ko-KR/README.md) | [번역 유지 관리](ko-KR/INTERNATIONALIZATION.md) | [GitHub 공개 및 기여](ko-KR/GITHUB.md) |
| 繁體中文 | [使用指南](zh-TW/README.md) | [國際化維護](zh-TW/INTERNATIONALIZATION.md) | [GitHub 發佈與協作](zh-TW/GITHUB.md) |

## Repository documents

Docker deployment guides: [English](en-US/DOCKER.md) · [简体中文](zh-CN/DOCKER.md) · [日本語](ja-JP/DOCKER.md) · [한국어](ko-KR/DOCKER.md) · [繁體中文](zh-TW/DOCKER.md). These describe source-built local images, persistent data, HTTPS, backups and upgrades.

| Need | Reference |
|---|---|
| Preview the English interface with synthetic demonstration data | [Screenshots](../README.md#screenshots) |
| Build, develop or submit a PR | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Find help or report a bug | [SUPPORT.md](../SUPPORT.md) |
| Report a vulnerability privately | [SECURITY.md](../SECURITY.md) |
| Understand changes and upgrade notes | [CHANGELOG.md](../CHANGELOG.md) |
| Review candidate future work | [ROADMAP.md](../ROADMAP.md) |
| Review license and attribution | [LICENSE](../LICENSE) · [NOTICE](../NOTICE) · [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) |
| Prepare release notes | [Release template](../.github/RELEASE_TEMPLATE.md) |
| Review repository-specific coding rules | [AGENTS.md (English)](../AGENTS.md) · [简体中文](zh-CN/COLLABORATION.md) |

## Design and historical evidence

The [Chinese design index](zh-CN/INDEX.md) separates implemented capabilities, design proposals and old-system scope mapping. [Historical validation records](zh-CN/VALIDATION_HISTORY.md) describe specific development snapshots, not guarantees for a future release. This design corpus is not fully translated; the five current guides describe the same supported behavior and boundaries.

Update affected current guides together when commands, prerequisites or behavior change. Update the indexes when adding or moving a document. Run `npm --prefix web run check:docs` to check English paths, case-sensitive local links and heading anchors, five-language navigation, and Markdown issue-template metadata. Run `npm --prefix web run test:docs` for checker regression tests. These offline checks require only Node.js; external URLs must be verified separately. Legal texts copied from upstream remain verbatim.
