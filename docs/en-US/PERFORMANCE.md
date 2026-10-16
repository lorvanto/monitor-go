# Performance verification

This is a development measurement record, not a published capacity guarantee. The initial acceptance workload is 20 hosts, 100 service checks and five concurrent viewers, targeting a 2-vCPU / 1-GiB Linux installation. Image-heavy installations should allow additional memory. Measurements on a Mac with `GOMAXPROCS=2` are not a substitute for a memory-limited Linux deployment test.

## Reproduce the baseline

```sh
MONITOR_PERF=1 GOMAXPROCS=2 go test ./internal/app -run '^TestPerformanceBaseline$' -count=1 -v
```

The opt-in test creates a disposable on-disk SQLite WAL database and an HTTP listener on a logged random loopback port. It seeds 1,008,000 check results, 806,400 host samples and 10,000 alert activities, rolls up completed hours and warms each endpoint. Five clients each make five sequential rounds of authenticated monitor, alert, overview and host-history requests. Concurrently, 40 loopback HTTP probes execute and persist their threshold-one alert. It checks alert persistence and SQLite integrity. No installation data or external probe targets are used.

The timed window excludes seeding and warm-up. It reports endpoint P50/P95, response bytes, process CPU time, sampled peak RSS, allocation volume, GC cycles and writer connection waits. The test process includes HTTP clients, server and SQLite; its RSS is not the standalone service's memory budget. The workload is a burst test, not five users refreshing every ten seconds. Probe-to-alert duration includes enqueue, probe and persistence, but not scheduler wake-up or notification transport. Those paths require separate integration coverage.

Use the same host, Go version, concurrency and dataset for comparisons. Heavy tests skip ordinary CI unless explicitly enabled. Profiles can be collected with Go test's `-cpuprofile` and `-memprofile` flags; do not expose profiling endpoints on a public installation.

## Results

Measurements and implementation progress are recorded in the [optimization plan](../zh-CN/PERFORMANCE_PLAN.md). Keep collection at 15 seconds, configured check intervals and alert thresholds unchanged while reducing redundant work. Existing authorization, unsaved drafts, language switching, APFS identity, notification suppression/retries and retention semantics remain regression requirements.

## Query and polling behavior

`GET /api/v1/poll?page=overview` (or another current page key) authenticates once and includes only resources allowed by that fresh identity, navigation and the open-alert badge. It retains the 32 check signal samples but omits their diagnostic text; `GET /api/v1/monitors/{id}/history` loads complete history for the open detail drawer with `monitor:read` authorization. It uses compact alert rows; `GET /api/v1/alerts/{id}/activity` loads at most 100 timeline entries with `alert:read` authorization. Existing `/monitors` and `/alerts` responses remain compatible; `/alerts?compact=1` explicitly omits activities.

Rolling 24-hour totals combine whole-hour summaries, the raw first partial hour and unconsumed results in a single SQLite snapshot, including late arrivals. Historical overview chart points use a bounded 32-entry, two-second cache and coalesce concurrent identical requests. Current check/host state, incidents, authentication and notification decisions are never served from that cache. No schema, durability, retention or sampling interval changes are required.

## Browser refresh

The UI uses one base page-poll request per ten-second foreground refresh. Overview adds one chart request (plus an overall chart when filtering a group); alert activity loads only for the selected event. Team data stays entry/mutation-driven. Hidden tabs pause polling and refresh immediately when visible again. Unchanged JSON subtrees retain references, shell/form subscriptions exclude unrelated metrics, and chart transforms are memoized. Host history reloads for a new sample or changed selection/range, with a minute boundary refresh when no sample arrives. Saving still schedules a refresh after an in-flight poll; stale sessions/pages cannot restore old data.

## Collection and worker scheduling

The collector refreshes OS/hostname/CPU-model inventory every five minutes. CPU, memory, IP addresses, network counters, disk capacity and verified APFS identity remain fresh each collection; uptime advances between inventory reads. Inventory refresh failures remain errors. The local 15-second cadence and Agent protocol are unchanged; update Agent binaries to benefit from inventory caching.

Check and notification workers use coalesced post-commit/completion wakeups and timers for the next durable due time, with a 30-second fallback for missed hints. Four probe workers, two notification workers, the five-second notification grace, configured check intervals, failure thresholds and retry backoffs are unchanged. SQLite is the source of truth; wakeups carry no jobs and cannot replace commit or restart recovery. Database failures back off rather than spinning.

## Maintenance and image peaks

Historical rollups commit at most 2,000 raw records per transaction. Summary accumulation and consumed markers remain atomic, including partial failures and late records. Startup/hourly maintenance runs independently of check dispatch. A cache image is opened under the generation/mutation lock and served after unlocking, so slow downloads do not hold up other cached reads or cache clearing. Generation remains serialized and dimension-limited.

Additional opt-in workloads:

```sh
# Current page-poll API instead of separate monitor/alert requests
MONITOR_PERF=1 MONITOR_PERF_POLL=1 GOMAXPROCS=2 go test ./internal/app -run '^TestPerformanceBaseline$' -count=1 -v
# Two concurrent preview requests for one synthetic 24-megapixel PNG
MONITOR_PERF=1 GOMAXPROCS=2 go test ./internal/app -run '^TestPerformanceImagePeak$' -count=1 -v
```

## Linux container comparison (2026-10-16)

Go 1.26.9, Linux arm64 on Docker Desktop; each disposable container limited to two CPUs, 1 GiB memory and no swap, `GOMAXPROCS=2`, networking disabled except loopback. Temporary SQLite files used an anonymous disposable disk volume, not the live data volume or a RAM filesystem. Before: commit `5c3aeda`; after: P1–P5 implementation. One run per variant, 25 requests per endpoint. These are measured synthetic workloads on a shared Mac host, not a long-running production capacity guarantee.

| Same four-endpoint burst | Before | After |
|---|---:|---:|
| Monitor list P95 | 1105.727ms | 134.955ms |
| Full legacy alert list P95 | 661.026ms | 152.917ms |
| Overview P95 | 1402.345ms | 103.083ms |
| Host history P95 | 343.720ms | 67.574ms |
| Probe-to-alert persistence P95 | 25.359ms | 20.149ms |
| Process CPU time | 22.500s | 2.600s |
| Timed wall duration | 12.218s | 1.409s |
| Test process peak RSS | 54.64 MiB | 43.70 MiB |

The current poll variant makes 75 total requests instead of 100: page-poll P95 91.242ms / mean response 281,757 bytes, overview 62.522ms, host history 36.617ms, probe-to-alert 8.057ms, CPU 1.270s, wall 1.226s, peak RSS 40.11 MiB. It changes the request mix and is not the identical-endpoint comparison above. Seeded host history represents 20 hosts; concurrent writes exercise the probe/alert lifecycle rather than 20 live Agents continuously reporting. External probe latency and external notification networks are excluded.

The separate 24MP image test completed two concurrent requests in 677ms with test-process peak RSS 317.09 MiB, including fixture generation and clients. Image peaks require more headroom than steady monitoring. Do not set a 100–200 MiB hard memory limit based on the monitoring-only results. No GC tuning or deployment memory cap was added.
