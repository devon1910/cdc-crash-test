# E2 active: quiet source: noise-only workload

Question: Does a heartbeat action query that updates a published table keep the slot advancing when only the noise table receives application writes?

## Configuration and versions
- Workload: 5999 committed `noise` inserts at 10/second for requested 10m0s (actual 10m0.002s); payload 4096 bytes. The runner wrote no `orders` rows.
- Observer: 121 samples at 5s; settle 5s; largest gap 10.001s (limit 12s).
- PostgreSQL: 17.11. Go: go1.25.5.
- Compose images:
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-stability-20260928-receiver`
- Slot: `cdc_crashtest_slot`. Heartbeat setting: heartbeat.interval.ms=10000 plus published-table action query.
- Heartbeat table updates: 60.

## Observations
- Sample window: 2026-09-28T19:02:22Z to 2026-09-28T19:12:29Z UTC.
- Slot active: true throughout. Debezium health: `UP` 121/121 samples.
- Confirmed flush LSN: `0/19322F0` to `0/1AAE760`; advanced: true.
- Restart LSN: `0/19322B8` to `0/1A86208`.
- Retained-WAL distance: start 424, end 354216, minimum 424, peak 375088 bytes; net change 353792 bytes (583.6 bytes/second).
- `pg_wal` directory size: 16777216 to 16777216 bytes. Checkpoint counter: 1 to 3.

## Verdict
The active heartbeat action query accompanied confirmed flush LSN advancement. Retained-WAL distance changed by 353792 bytes. Compare this trend with the separately reset E1 and timer-only runs; LSN progress alone does not prove sink delivery.
