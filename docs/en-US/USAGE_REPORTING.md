# Optional usage reporting

[English](../en-US/USAGE_REPORTING.md) · [简体中文](../zh-CN/USAGE_REPORTING.md) · [日本語](../ja-JP/USAGE_REPORTING.md) · [한국어](../ko-KR/USAGE_REPORTING.md) · [繁體中文](../zh-TW/USAGE_REPORTING.md)

Monitor Go and Monitor Console are independent repositories and services. Keep them in sibling directories with separate data directories. Monitor Go continues to work without the console.

When maintaining both projects, place their checkouts in sibling directories with separate data directories. A normal Monitor Go installation needs only this public repository; optional reporting requires a reachable console service, not access to its private source repository.

Reporting is disabled on fresh installations and when upgrading from a version without this feature. Later restarts and upgrades preserve an administrator’s saved choice for the same recipient. Configure the console origin on the server, then an administrator with system:settings can review the fields and explicitly enable reporting in System settings. Configuring a URL alone does not enable reporting. Changing the recipient origin requires new consent.

Reports contain a random installation ID, version, OS family, CPU architecture, deployment mode and UTC dates on which a signed-in person interacted with the interface. They exclude local account lists, monitored addresses, host names/IPs, files, alert text, passwords, monitoring keys and application keys. The instance credential is sent only to authenticate registration/reporting, never displayed. Console sign-in separately records the voluntarily connected GitHub ID, login and display name.

Sign in to Monitor Console with GitHub, generate a ten-minute single-use binding code, then paste it into the reporting card in Monitor Go. Reporting must already be enabled. Unlinking removes the account association; it does not disable reporting. You can also unlink your own installations from the console.

“Last synced GitHub account” is a locally saved snapshot. After unlinking in Monitor Console, Monitor Go updates it only after the next successful report. Reporting stays disconnected from the console while disabled, so the previous account may remain visible. “Refresh reporting status” reloads local settings only; it does not contact the console or start a remote synchronization.

Reporting runs independently after enabling, then approximately every six hours with jitter. Requests time out after five seconds, reject redirects and retry with bounded backoff. At most seven UTC active-day flags are retained for delivery. Polling and page loads alone do not count as human activity. Daily aggregates count distinct installations, not individual people. Reports are self-reported adoption estimates, not proof of identity or exact installation totals.

Turn reporting off at any time: in-flight requests are cancelled and queued activity is cleared. Monitoring, alerts and login remain independent. Disabling does not erase historical data already received by the console. Daily records there expire after 90 days and installations after 365 days without reports. A later report may register an expired installation again; account binding must then be renewed.

Use HTTPS; numeric loopback HTTP is permitted only for local integration. No official console domain is configured by default. Docker containers cannot reach a host service through their own loopback; use a real HTTPS endpoint for separate containers. Never publish credentials, data directories or backups to Git. Configure reverse-proxy access logs to avoid OAuth query strings, full source IPs and credentials.

Schema 9 adds reporting settings and coarse activity dates. Back up the entire data directory before upgrading; rollback requires the previous full backup. The random identity persists with that data directory; cloning it can merge statistics. Disabled/offline/modified installations cannot be counted completely.

```sh
MONITOR_CONSOLE_URL=https://console.example.com ./bin/monitor -data data -listen 127.0.0.1:5080
```

`MONITOR_CONSOLE_URL` / `-console-url`; `MONITOR_DEPLOYMENT=native|docker` / `-deployment`.

The native binary defaults to `native`; the Docker image and Compose set `docker` automatically.

[README](README.md) · [Docker](DOCKER.md)
