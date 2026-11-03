# Screenshot assets

These are captures of the running Monitor Go interface, not rendered mockups. The root README uses Simplified Chinese. Each current user guide links to a gallery in the same language:

[简体中文](../../zh-CN/SCREENSHOTS.md) · [English](../../en-US/SCREENSHOTS.md) · [日本語](../../ja-JP/SCREENSHOTS.md) · [한국어](../../ko-KR/SCREENSHOTS.md) · [繁體中文](../../zh-TW/SCREENSHOTS.md)

## Capture record

- Captured on 2026-11-03 from application commit `2bc6819`, schema 8, using the light theme and a desktop browser viewport (1280 × 720).
- Five isolated local installations contain synthetic hosts, metrics, checks, incidents, folders, applications, key prefixes and package records. Local host collection is disabled. No real infrastructure, user uploads, secrets or private logs appear in the images.
- Demo resource names and descriptions are localized within the fixtures. Protocol names, example domains, filenames, technical IDs and the fictional member Alex Morgan remain literal. This does not change the application's handling of user-authored text.
- Application keys use nonfunctional demo prefixes. Package records are illustrative and do not announce public Monitor Go releases. Metrics are not performance measurements.
- The files are original browser captures, with no text replacement or compositing. Pages can extend below the viewport or expose additional columns by scrolling.

## Maintenance

Keep the same nine filenames in each locale directory: `overview.jpg`, `hosts.jpg`, `service-checks.jpg`, `alerts.jpg`, `files.jpg`, `applications.jpg`, `packages.jpg`, `permissions.jpg` and `code-generator.jpg`.

Capture each page with its corresponding UI language selected. Use a separate temporary data directory and `-collect-host=false`; never use a real installation's database or credentials. Wait for data, charts and layout to finish rendering. Inspect the actual image for clipped headings, missing navigation, loading states and mixed system languages before publication. Update all five galleries and their captions when the displayed scope changes. Keep runtime fixtures, passwords and capture logs out of Git.

Run `npm --prefix web run check:docs` and verify every linked JPEG exists. These screenshots document interface appearance, not platform certification or operational capacity.
