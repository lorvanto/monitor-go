# First activation and usage reporting

[English](../en-US/USAGE_REPORTING.md) · [简体中文](../zh-CN/USAGE_REPORTING.md) · [日本語](../ja-JP/USAGE_REPORTING.md) · [한국어](../ko-KR/USAGE_REPORTING.md) · [繁體中文](../zh-TW/USAGE_REPORTING.md)

Monitor Go and Monitor Console are independent repositories, services and databases. Keep their source and data directories separate. A Monitor Go installation needs only this public source repository, not access to the private Console source. Standard first setup contacts the configured Console for activation; subsequent login, monitoring and alerts run independently.

Standard new installations require online activation before the first local administrator is created. Setup shows the destination (preset to `https://feiniu.i7f.cn:5090`), purpose, exact fields and the rule that reporting remains enabled with no off switch. The installer must explicitly confirm disclosure version `2`. Successful registration creates the local administrator and enables continuous background reporting. No local username, display name or password is sent to Console. Registration failure creates no administrator; retry with the same installation identity after restoring connectivity. Activation does not require a Console account or GitHub sign-in and never automatically links a developer account.

Already enabled installations keep their identity, credential, account link and historical consent on upgrade; the upgrade does not fabricate a new confirmation. Legacy installations with reporting off are not silently enabled. An administrator with `system:settings` must review the current destination, fields and no-off-switch rule, then confirm once; only successful registration enables reporting. Registration or transaction failure leaves it off. Local login, monitoring and alerts continue while confirmation is pending. Changing the recipient clears the previous local reporting identity and requires confirmation for the new destination before any report is sent there.

Reports contain a random installation ID, version, OS family, CPU architecture, deployment mode and UTC dates on which a signed-in person interacted with the interface. They exclude local account lists, monitored addresses, host names/IPs, files, alert text, passwords, monitoring keys and application keys. The instance credential is sent only to authenticate registration/reporting, never displayed. Console accounts are separate from Monitor Go. A local Console account stores its username and display name; optional GitHub sign-in separately stores the verified GitHub ID, login and display name.

Installation registration and activity statistics do not require account binding. Console supports a local administrator username/password; GitHub sign-in is optional. Installing Monitor Go does not require OAuth configuration.

To optionally associate an account, sign in to Monitor Console with a local Console account or enabled GitHub sign-in, generate a ten-minute single-use binding code, then paste it into the reporting card in Monitor Go. Reporting must already be enabled. Unlinking removes the account association; it does not disable reporting. You can also unlink your own installations from the console.

“Last synced Console account” is a locally saved snapshot. Unlinking in Monitor Console is reflected in Monitor Go after the next successful report. During connectivity failures or pending confirmation on a legacy installation, an older account may remain visible. “Refresh reporting status” only reloads local settings; it does not request a remote synchronization.

Reporting runs independently after enabling, then approximately every six hours with jitter. Requests time out after five seconds, reject redirects and retry with bounded backoff. At most seven UTC active-day flags are retained for delivery. Polling and page loads alone do not count as human activity. Daily aggregates count distinct installations, not individual people. Reports are self-reported adoption estimates, not proof of identity or exact installation totals.

If remote registration succeeds but local setup fails or is cancelled, the console may retain an unlinked random installation record. Retrying reuses the same identity. Registered installations therefore do not equal completed installations or individual people.

After activation or confirmation, reporting stays enabled and cannot be turned off through the interface, API or deployment flags; a request to disable it returns HTTP 403. Console outages use independent background retries and never block login, monitoring or alerts. Daily records at Console expire after 90 days and installations after 365 days without reports. A later report can register an expired installation again, requiring a new account link. Unlinking an account does not stop reports or erase received history.

Use HTTPS; numeric loopback HTTP is permitted only for local integration. Docker containers cannot reach a host service through their own loopback; use a real HTTPS endpoint for separate containers. Never publish credentials, data directories or backups to Git. Configure reverse-proxy access logs to avoid OAuth query strings, full source IPs and credentials.

The database remains at schema 10. Disclosure version `2` records confirmation of mandatory ongoing reporting using the existing notice version, consent time and activation time fields. Existing historical consent is not rewritten. Back up the entire data directory before upgrading; rollback requires the previous full backup. The random identity persists with the data directory and cloning it can merge statistics. Unconfirmed legacy installations and unreachable clients mean statistics are not a complete installation census.

When `MONITOR_CONSOLE_URL` is unset, the native binary and Compose use `https://feiniu.i7f.cn:5090`. Set another HTTPS origin to change the destination. The native `-console-url` flag takes precedence. Empty or whitespace-only destinations are invalid configuration and rejected by the standard CLI, including when supplied through the environment; they cannot disable reporting or bypass activation. The version and backup commands do not require activation configuration.

Standard CLI and Compose deployments require activation and reporting. The old `-require-activation=false` and `MONITOR_REQUIRE_ACTIVATION=false` settings are rejected with a configuration error; `true` remains accepted for compatibility. There is no supported offline-installation or reporting-disable flag. Configure a reachable HTTPS Console destination; empty or whitespace-only destinations are invalid startup configuration and cannot bypass activation. The Console URL flag overrides its environment value; an activation=false value cannot be made valid by adding an activation=true flag.

```sh
./bin/monitor -data data -listen 127.0.0.1:5080
```

`MONITOR_CONSOLE_URL` / `-console-url`; `MONITOR_REQUIRE_ACTIVATION` / `-require-activation`; `MONITOR_DEPLOYMENT=native|docker` / `-deployment`.

The native binary defaults to `native`; the Docker image and Compose set `docker` automatically.

[README](README.md) · [Docker](DOCKER.md)
