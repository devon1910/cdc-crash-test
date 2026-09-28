# E2 timer-only: quiet source: noise-only workload

Question: Is heartbeat.interval.ms alone enough to advance the slot when only the unpublished noise table changes?

## Configuration and versions
- Workload: 5999 committed `noise` inserts at 10/second for requested 10m0s (actual 10m0.001s); payload 4096 bytes. The runner wrote no `orders` rows.
- Observer: 121 samples at 5s; settle 5s; largest gap 10s (limit 12s).
- PostgreSQL: 17.11. Go: go1.25.5.
- Compose images:
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-m4-comparison-2-receiver`
- Slot: `cdc_crashtest_slot`. Heartbeat setting: heartbeat.interval.ms=10000; no action query.
- Heartbeat table updates: 0.

## Observations
- Sample window: 2026-09-28T14:18:03Z to 2026-09-28T14:28:08Z UTC.
- Slot active: true throughout. Debezium health: `UP` 121/121 samples.
- Confirmed flush LSN: `0/19322F0` to `0/19322F0`; advanced: false.
- Restart LSN: `0/19322B8` to `0/19322B8`.
- Retained-WAL distance: start 96, end 1918712, minimum 96, peak 1918712 bytes; net change 1918616 bytes (3167.3 bytes/second).
- `pg_wal` directory size: 16777216 to 16777216 bytes. Checkpoint counter: 1 to 3.

## Verdict
The 10-second timer-only heartbeat did not advance the confirmed flush LSN while only unpublished `noise` rows were written. Retained-WAL distance changed by 1918616 bytes. Under this setup, timer-only did not prevent the quiet-source stall.
