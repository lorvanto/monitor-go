# Translation maintenance

**English** · [简体中文](../zh-CN/INTERNATIONALIZATION.md) · [日本語](../ja-JP/INTERNATIONALIZATION.md) · [한국어](../ko-KR/INTERNATIONALIZATION.md) · [繁體中文](../zh-TW/INTERNATIONALIZATION.md) · [Documentation](../README.md)

## Shared catalogs and code

`locales/{zh-CN,en-US,ja-JP,ko-KR,zh-TW}.json` is the shared source for Go and React. Go embeds it with `go:embed`; React loads non-English catalogs on demand. Source strings act as gettext-style IDs. `MessageKey` checks frontend keys at compile time. The catalog checker verifies identical key sets and interpolation tokens and rejects untranslated visible template text.

`web/src/i18n/locale.ts` contains locale matching and interpolation. `index.ts` holds reactive state and guarded local storage. `LanguageSelect.tsx` is shared by setup/sign-in and navigation. API types and display formatting are separated into `lib/types.ts` and `format.ts`. Business state stays in `stores/app.ts`, which re-exports the extracted types and helpers for compatibility. `internal/app/localization.go` integrates Go request language, formatted errors and built-in menu keys without a schema change.

## Contracts

- Supported locales: `zh-CN`, `en-US`, `ja-JP`, `ko-KR`, `zh-TW`. A new browser starts in English, regardless of its language preferences. Restore a valid saved `monitor.locale` choice; missing or invalid preferences and unavailable storage use English. Traditional Chinese includes Hant, TW, HK and MO tags.
- Keep query values, permission keys, endpoint paths and stored data independent of display text. Time ranges use `1h`, `24h`, `7d`. Permanent deletion sends the fixed `DELETE` token after the UI validates the translated confirmation phrase; the server also accepts the legacy Chinese phrase.
- Use `t()` for static UI messages and entire sentences with `{p0}`, `{p1}` parameters. Do not concatenate translated sentence fragments. Interpolation produces plain strings rendered by React, never HTML.
- Static label maps use `localizedLabels()` to remain reactive. `metadataLabel()` is only for trusted fixed permission/scope labels. Never apply it indiscriminately to server records or user text.
- Default built-in menu names have an optional `nameKey`; renamed/custom menus remain literal. Editing forms retain raw values. Persisted diagnostics, alerts, audit history, notification payloads, logs and CLI help retain their original language.
- Frontend JSON and upload requests carry `Accept-Language`. API negotiation respects quality weights. Responses expose `Content-Language` and `Vary`. A missing header preserves the original Chinese behavior; an unsupported header falls back to English. Status codes and response field names do not change. Clients must not parse translated error prose.
- Parameterized Go validation uses `badf()` so `%d` placeholders are formatted after language selection. Keep placeholder counts consistent across catalogs.
- `Intl` handles numbers, relative time and dates using the browser's local time zone. Switching language must not reload a page, reset an open form or submit a business action.

Traditional Chinese is an independent catalog. Initial script conversion was assisted by system tooling with terminology adjustments; ongoing edits should treat it as a translation. All five catalogs have full key coverage; native-language editorial review remains useful.

## Add a language

1. Copy a complete catalog and translate every value, preserving keys and placeholders.
2. Register the BCP 47 code in Go `Supported`/`Match`, frontend locale metadata/matching and the dynamic loader map.
3. Update `scripts/check-i18n.mjs`, tests, native language names and documentation navigation. Add an actual current guide. For RTL languages, add direction and layout support; current locales are LTR.
4. Verify sign-in, preference persistence, forms, default/custom menus, upload errors, API errors and narrow layouts in an isolated instance.

```sh
npm --prefix web run check:i18n
npm --prefix web run check:docs
npm --prefix web run test:docs
npm --prefix web test
npm --prefix web run typecheck
npm --prefix web run build
npm --prefix web run format:check
go test ./...
go test -race ./...
go vet ./...
```

`prettier 3.6.2` is a pinned development-only formatter. Existing runtime dependencies and lockfiles are retained. `make test` also runs frontend locale tests and Go race/vet checks. HTTP tests use real temporary SQLite files; never copy production data into a language test fixture.

## Documentation policy

The root README is the English entry point. All five languages use `docs/<locale>/`, with English directory and file names and localized content. Simplified Chinese material lives in `docs/zh-CN/`, and Traditional Chinese guides in `docs/zh-TW/`. The original Chinese architecture and historical planning corpus remains authoritative for its stated scope; it is not claimed to be fully translated or implemented. Keep all current guides aligned when commands, prerequisites or product boundaries change. Update the Chinese index for new Chinese documents.

## Localization verification on 2026-10-14, before GitHub documentation preparation

All five catalogs contain 1152 messages with consistent keys and placeholders. The build also runs `scripts/check-docs.mjs`: 22 Markdown files and 92 local file links passed. Type checking, production build, frontend tests, Prettier, Go formatting, `go test ./...`, race tests, vet, and server/Agent builds passed.

An isolated SQLite instance on `127.0.0.1:5093` passed 20 live HTTP checks across five languages for authentication, parameterized validation, empty uploads and unknown routes. Browser checks covered 12 pages in five languages on desktop and at 390px width (120 page checks), with no page-level horizontal overflow. Preference persistence, unsaved settings input, Japanese validation, multilingual QR generation and a narrow file dialog were also checked. Production data was not used; these checks do not replace native-language editorial review.
