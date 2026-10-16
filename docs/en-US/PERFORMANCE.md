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

`GET /api/v1/poll?page=overview` (or another current page key) authenticates once and includes only resources allowed by that fresh identity, navigation and the open-alert badge. It uses compact alert rows; `GET /api/v1/alerts/{id}/activity` loads at most 100 timeline entries with `alert:read` authorization. Existing `/monitors` and `/alerts` responses remain compatible; `/alerts?compact=1` explicitly omits activities.

Rolling 24-hour totals combine whole-hour summaries, the raw first partial hour and unconsumed results in a single SQLite snapshot, including late arrivals. Historical overview chart points use a bounded 32-entry, two-second cache and coalesce concurrent identical requests. Current check/host state, incidents, authentication and notification decisions are never served from that cache. No schema, durability, retention or sampling interval changes are required.

## Browser refresh

The UI uses one base page-poll request per ten-second foreground refresh. Overview adds one chart request (plus an overall chart when filtering a group); alert activity loads only for the selected event. Team data stays entry/mutation-driven. Hidden tabs pause polling and refresh immediately when visible again. Unchanged JSON subtrees retain references, shell/form subscriptions exclude unrelated metrics, and chart transforms are memoized. Host history reloads for a new sample or changed selection/range, with a minute boundary refresh when no sample arrives. Saving still schedules a refresh after an in-flight poll; stale sessions/pages cannot restore old data.

## Collection and worker scheduling

The collector refreshes OS/hostname/CPU-model inventory every five minutes. CPU, memory, IP addresses, network counters, disk capacity and verified APFS identity remain fresh each collection; uptime advances between inventory reads. Inventory refresh failures remain errors. The local 15-second cadence and Agent protocol are unchanged; update Agent binaries to benefit from inventory caching.

Check and notification workers use coalesced post-commit/completion wakeups and timers for the next durable due time, with a 30-second fallback for missed hints. Four probe workers, two notification workers, the five-second notification grace, configured check intervals, failure thresholds and retry backoffs are unchanged. SQLite is the source of truth; wakeups carry no jobs and cannot replace commit or restart recovery. Database failures back off rather than spinning.
