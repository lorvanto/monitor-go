# Roadmap

The [README](README.md) and current guides describe implemented behavior. The items below are candidates for future work, not a release commitment or a list of available features. Priorities are confirmed in maintainer-approved issues.

| Area | Next work to consider | Evidence needed before claiming completion |
|---|---|---|
| First public release | Confirm release version, supported targets and repeatable packaging | Clean-checkout build, target-machine smoke tests, checksums and complete notices |
| Linux deployment | Service and Agent runtime verification on intended architectures | Real-machine sampling, restart/reconnect and TLS checks |
| Operations | Backup/restore tools and measured capacity guidance | Recovery rehearsal and workload/hardware-specific results |
| Notifications | Resource routing, maintenance windows and repeat reminders | Delivery, suppression, retry and duplicate-handling tests |
| Certificates | ACME issuance, validation and renewal workflows | Staging issuance, failure recovery and private-key protections |
| Agent lifecycle | Version compatibility, packaging and update workflow | Upgrade/rollback, credential preservation and platform tests |
| Documentation | Native-language editorial review and more deployment examples | Synchronized commands and verified examples in the current guides |

SQLite-only storage, shared installation resources and server-side permission checks remain project boundaries. New integrations do not automatically justify external infrastructure dependencies or multi-tenancy.

The [Chinese design index](docs/zh-CN/INDEX.md) contains detailed proposals and historical scope mapping. Where a proposal differs from current code, treat it as planned work. See [release preparation](docs/en-US/RELEASING.md) for the publication checklist.
