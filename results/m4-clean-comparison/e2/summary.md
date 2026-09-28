# E2 active: quiet source: noise-only workload

Question: Does a heartbeat action query that updates a published table keep the slot advancing when only the noise table receives application writes?

## Configuration and versions
- Workload: 5999 committed `noise` inserts at 10/second for requested 10m0s (actual 10m0.008s); payload 4096 bytes. The runner wrote no `orders` rows.
- Observer: 122 samples at 5s; settle 5s; largest gap 6.212s (limit 12s).
- PostgreSQL: 17.11. Go: go1.25.5.
- Compose images:
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-m4-comparison-2-receiver`
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`
- Slot: `cdc_crashtest_slot`. Heartbeat setting: heartbeat.interval.ms=10000 plus published-table action query.
- Heartbeat table updates: 59.

## Observations
- Sample window: 2026-09-28T14:28:45Z to 2026-09-28T14:38:51Z UTC.
- Slot active: true throughout. Debezium health: `UP` 122/122 samples.
- Confirmed flush LSN: `0/19322F0` to `0/1AEC840`; advanced: true.
- Restart LSN: `0/19322B8` to `0/1AC4018`.
- Retained-WAL distance: start 368, end 327320, minimum 368, peak 598888 bytes; net change 326952 bytes (539.3 bytes/second).
- `pg_wal` directory size: 16777216 to 16777216 bytes. Checkpoint counter: 1 to 3.

## Verdict
The active heartbeat action query accompanied confirmed flush LSN advancement. Retained-WAL distance changed by 326952 bytes. Compare this trend with the separately reset E1 and timer-only runs; LSN progress alone does not prove sink delivery.
