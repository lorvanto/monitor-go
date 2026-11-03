# First activation and usage reporting

[English](../en-US/USAGE_REPORTING.md) · [简体中文](../zh-CN/USAGE_REPORTING.md) · [日本語](../ja-JP/USAGE_REPORTING.md) · [한국어](../ko-KR/USAGE_REPORTING.md) · [繁體中文](../zh-TW/USAGE_REPORTING.md)

Monitor Go and Monitor Console are independent repositories, services and databases. Keep their source and data directories separate. A Monitor Go installation needs only this public source repository, not access to the private Console source. Standard first setup contacts the configured Console for activation; subsequent login, monitoring and alerts run independently.

Standard new installations require online activation before the first local administrator is created. Setup shows the destination (preset to `https://feiniu.i7f.cn:5090`), the reporting purpose and exact fields, and requires an explicit confirmation checkbox. Registration succeeds before administrator creation and reporting is then enabled. Setup does not send the local username, display name or password. If registration fails, no administrator is created; retry with the same installation identity after fixing connectivity. No Console account, GitHub login or developer account binding is required.

An already initialized installation is not forced to activate on upgrade, and its saved reporting choice is not silently enabled or changed for the same recipient. An old installation without reporting remains off. Administrators with `system:settings` may review and enable it later. Changing the destination requires new consent. After activation, reporting can still be turned off; Console outages never prevent login, monitoring or alerts.

Reports contain a random installation ID, version, OS family, CPU architecture, deployment mode and UTC dates on which a signed-in person interacted with the interface. They exclude local account lists, monitored addresses, host names/IPs, files, alert text, passwords, monitoring keys and application keys. The instance credential is sent only to authenticate registration/reporting, never displayed. Console accounts are separate from Monitor Go. A local Console account stores its username and display name; optional GitHub sign-in separately stores the verified GitHub ID, login and display name.

Installation registration and activity statistics do not require account binding. Console supports a local administrator username/password; GitHub sign-in is optional. Installing Monitor Go does not require OAuth configuration.

To optionally associate an account, sign in to Monitor Console with a local Console account or enabled GitHub sign-in, generate a ten-minute single-use binding code, then paste it into the reporting card in Monitor Go. Reporting must already be enabled. Unlinking removes the account association; it does not disable reporting. You can also unlink your own installations from the console.

“Last synced Console account” is a locally saved snapshot. After unlinking in Monitor Console, Monitor Go updates it only after the next successful report. Reporting stays disconnected from the console while disabled, so the previous account may remain visible. “Refresh reporting status” reloads local settings only; it does not contact the console or start a remote synchronization.

Reporting runs independently after enabling, then approximately every six hours with jitter. Requests time out after five seconds, reject redirects and retry with bounded backoff. At most seven UTC active-day flags are retained for delivery. Polling and page loads alone do not count as human activity. Daily aggregates count distinct installations, not individual people. Reports are self-reported adoption estimates, not proof of identity or exact installation totals.

If remote registration succeeds but local setup fails or is cancelled, the console may retain an unlinked random installation record. Retrying reuses the same identity. Registered installations therefore do not equal completed installations or individual people.

Turn reporting off at any time: in-flight requests are cancelled and queued activity is cleared. Monitoring, alerts and login remain independent. Disabling does not erase historical data already received by the console. Daily records there expire after 90 days and installations after 365 days without reports. A later report may register an expired installation again; account binding must then be renewed.

Use HTTPS; numeric loopback HTTP is permitted only for local integration. Docker containers cannot reach a host service through their own loopback; use a real HTTPS endpoint for separate containers. Never publish credentials, data directories or backups to Git. Configure reverse-proxy access logs to avoid OAuth query strings, full source IPs and credentials.

Schema 10 adds the disclosure version, consent time and activation time to the reporting settings introduced in schema 9. Back up the entire data directory before upgrading; rollback requires the previous full backup. The random identity persists with that data directory; cloning it can merge statistics. Disabled/offline/modified installations cannot be counted completely.

When `MONITOR_CONSOLE_URL` is unset, the native binary and Compose use `https://feiniu.i7f.cn:5090`. Set it to another HTTPS origin to override the destination, or explicitly set `MONITOR_CONSOLE_URL=` to leave it unconfigured. The native `-console-url` flag takes precedence over the environment; `-console-url=` also leaves the destination unconfigured.

The standard CLI and Compose default to `MONITOR_REQUIRE_ACTIVATION=true` / `-require-activation=true`. For a self-built distribution or an offline environment, explicitly set `MONITOR_REQUIRE_ACTIVATION=false` or pass `-require-activation=false`; setup then remains local and reporting stays off until separately enabled. An empty Console URL does not bypass required activation: configure a valid destination or explicitly disable the requirement. Command-line flags take precedence over environment variables.

```sh
./bin/monitor -data data -listen 127.0.0.1:5080
```

Example for a deliberately offline installation:

```sh
./bin/monitor -data data -listen 127.0.0.1:5080 -require-activation=false -console-url=
```

`MONITOR_CONSOLE_URL` / `-console-url`; `MONITOR_REQUIRE_ACTIVATION` / `-require-activation`; `MONITOR_DEPLOYMENT=native|docker` / `-deployment`.

The native binary defaults to `native`; the Docker image and Compose set `docker` automatically.

[README](README.md) · [Docker](DOCKER.md)
