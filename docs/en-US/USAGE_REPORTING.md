# First activation and anonymous installation statistics

[English](../en-US/USAGE_REPORTING.md) · [简体中文](../zh-CN/USAGE_REPORTING.md) · [日本語](../ja-JP/USAGE_REPORTING.md) · [한국어](../ko-KR/USAGE_REPORTING.md) · [繁體中文](../zh-TW/USAGE_REPORTING.md)

Monitor Go and Monitor Console are independent repositories, services and databases. Keep their source and data directories separate. Installing Monitor Go needs only this public repository, not private Console source. First setup registers with the configured Console; subsequent local login, monitoring and alerts run independently.

Standard first setup requires online activation before creating the first local administrator. The page shows the exact recipient, purpose, fields and mandatory ongoing reporting rule with no off switch, and requires explicit notice-version-2 confirmation. A failed registration creates no administrator; retries reuse the random installation identity. Product users do not need Console accounts.

Statistics cover distinct installations, active installations and product-version/OS distributions. A v2 registration contains `instanceId`, `credential`, `version` and `os`; reports contain `instanceId`, `version`, `os` and `activeDays`. The random ID deduplicates installations, the separate credential authenticates requests, and activity is grouped by UTC date. Versions and OS families are technical distributions with no product-user account association. Architectures, deployment types, local members, host names/IPs, monitoring content, files, passwords and monitoring/application keys are not sent. Anonymous statistics use a fixed User-Agent without a build version.

Product-user account binding, binding codes, account lists and My installations have been retired. Console retains only its own administrator authentication records for managing the statistics service; those administrators are not a product-user count. Monitor Go local administrators, members and authorization continue independently.

Counts deduplicate random installation identities, not individual people. Several people sharing one installation still count once; cloning a data directory can merge counts, and creating a fresh directory can create another installation. A successful remote registration can remain if local setup fails or is cancelled, so registrations do not necessarily mean completed installations.

The independent worker reports after activation, then approximately every six hours with jitter. Requests time out after five seconds, reject redirects and retry with bounded backoff. At most seven pending UTC active dates are retained; automatic refreshes and page loads alone are not activity. Console keeps daily records for 90 days and removes installations silent for 365 days; later reports can register again.

With an unchanged recipient, upgrades preserve the random identity, credential, enabled state, historical consent and active dates. Reducing fields does not fabricate new consent; notice version remains `2`. Legacy off installations require administrator confirmation of the current recipient and rule, followed by successful registration. Pending confirmation and Console outages do not interrupt local login, monitoring or alerts. A changed recipient requires confirmation before sending there. Ongoing reporting still has no off switch; disable requests return HTTP 403.

Upgrade Monitor Console to schema `3` before Monitor Go to schema `11`. New clients use `/api/v2/instances/register` and `/api/v2/instances/report`. Console v1 registration/reporting remains compatible with old clients: version and OS family are retained, while architecture and deployment fields are discarded without storing or returning them. Binding endpoints are retired. Monitor Go v11 physically removes the former `usage_settings.account_login` snapshot; Console v3 removes architectures, deployment types, account associations and binding codes while retaining version/OS distributions and anonymous counts.

Stop services normally and make separate complete backups of both data directories before upgrading. Rollback requires each prior image/binary and its complete pre-upgrade backup; never share the databases. Older backups may still contain retired metadata and must remain private. Existing confirmed installations with an unchanged recipient do not require clearing data and activating again.

Without `MONITOR_CONSOLE_URL`, the built-in preset HTTPS origin is used; inspect the setup or confirmation page for the actual address. An explicit environment value overrides the preset and `-console-url` overrides the environment. To retain a previous backend, explicitly configure its HTTPS origin before upgrading. Empty or whitespace-only destinations are invalid. Former `-require-activation=false` / `MONITOR_REQUIRE_ACTIVATION=false` settings are rejected; `true` remains compatible. Version and backup commands do not depend on activation settings.

HTTPS is required; numeric-loopback HTTP is only for isolated integration tests. Never commit credentials, data directories or backups, and do not retain full source IPs or credentials in statistics-proxy logs. `MONITOR_DEPLOYMENT` / `-deployment` still identifies the local runtime mode but is no longer sent as anonymous statistics.

```sh
./bin/monitor -data data -listen 127.0.0.1:5080
```

[README](README.md) · [Docker](DOCKER.md)
