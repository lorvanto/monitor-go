# Support

Start with the [five-language user guides](docs/README.md). Use GitHub Issues for reproducible bugs and focused feature proposals in [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go/issues). Discussions, chat groups and paid support are not currently configured. Security reports follow [SECURITY.md](SECURITY.md).

## Before opening an issue

Include the application version or commit, OS/architecture, Go and Node.js versions if building from source, browser/version, selected interface language, installation method and exact steps. State the expected and actual result. Include only the relevant redacted log excerpt or a screenshot using synthetic data. Do not attach runtime directories, cookies, keys, database files or complete Agent configuration.

## Common problems

| Symptom | What to check |
|---|---|
| Go reports missing embedded web assets | Run `make build`, or `npm --prefix web ci` followed by `npm --prefix web run build`, before Go build/test. |
| Frontend changes do not appear in the binary | Rebuild and restart the Go server; changing `web/src` alone does not replace embedded assets. |
| Port 5188 is occupied | Stop the conflicting development server. Vite uses a strict port to keep origin checks predictable. |
| Development API calls reach the wrong service | Set `MONITOR_GO_API_TARGET` explicitly and verify that the intended isolated backend is listening on that port. |
| Login/write requests fail behind a proxy | Check the browser origin against `-public-url`, HTTPS and `-secure-cookie`, and refresh stale sessions/CSRF state. |
| A data-directory lock prevents startup | Ensure only one active server uses that directory. Do not delete a lock while another process is running. |
| An Agent remains offline | Check enrollment expiry, valid TLS/CA configuration, connectivity and clock synchronization. Do not reuse another host's credentials. |
| A file cannot be removed | Check default-folder restrictions, permissions and package references; archive the referencing package before deleting its file. |
| Settings or role changes conflict | Reload the latest record before editing; HTTP 409 protects another member's changes. |
| A saved name or old incident stays in Chinese | User data and historical records retain their original language. The selector translates interface text, not stored content. |
| A language package fails to load | Retry from the selector and verify the server serves the newly built assets. Browser language preferences are local to that browser. |

The configured limits of 500 checks and 1000 remote hosts are input limits, not measured capacity guarantees. Include workload size and sampling frequency when reporting performance problems. Maintainers do not promise a response time.
