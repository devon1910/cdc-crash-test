# E1: quiet source, no heartbeat: noise-only workload

Question: When only the unpublished noise table changes and heartbeats are disabled, does a healthy Debezium slot continue to advance?

## Configuration and versions
- Workload: 5999 committed `noise` inserts at 10/second for requested 10m0s (actual 10m0.023s); payload 4096 bytes. The runner wrote no `orders` rows.
- Observer: 122 samples at 5s; settle 5s; largest gap 5.356s (limit 12s).
- PostgreSQL: 17.11. Go: go1.25.5.
- Compose images:
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-stability-20260928-receiver`
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`
- Slot: `cdc_crashtest_slot`. Heartbeat setting: disabled.
- Heartbeat table updates: 0.

## Observations
- Sample window: 2026-09-28T19:15:48Z to 2026-09-28T19:25:54Z UTC.
- Slot active: true throughout. Debezium health: `UP` 122/122 samples.
- Confirmed flush LSN: `0/19322B8` to `0/19322B8`; advanced: false.
- Restart LSN: `0/1932280` to `0/1932280`.
- Retained-WAL distance: start 152, end 1899872, minimum 152, peak 1899872 bytes; net change 1899720 bytes (3138.1 bytes/second).
- `pg_wal` directory size: 16777216 to 16777216 bytes. Checkpoint counter: 1 to 3.

## Verdict
With heartbeats disabled, the confirmed flush LSN stayed fixed while Debezium remained healthy and active. Retained-WAL distance changed by 1899720 bytes. This run reproduced the quiet-source stall.
