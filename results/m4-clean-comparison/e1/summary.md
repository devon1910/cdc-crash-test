# E1: quiet source, no heartbeat: noise-only workload

Question: When only the unpublished noise table changes and heartbeats are disabled, does a healthy Debezium slot continue to advance?

## Configuration and versions
- Workload: 5999 committed `noise` inserts at 10/second for requested 10m0s (actual 10m0.001s); payload 4096 bytes. The runner wrote no `orders` rows.
- Observer: 121 samples at 5s; settle 5s; largest gap 9.998s (limit 12s).
- PostgreSQL: 17.11. Go: go1.25.5.
- Compose images:
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-m4-comparison-2-receiver`
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`
- Slot: `cdc_crashtest_slot`. Heartbeat setting: disabled.
- Heartbeat table updates: 0.

## Observations
- Sample window: 2026-09-28T14:07:09Z to 2026-09-28T14:17:15Z UTC.
- Slot active: true throughout. Debezium health: `UP` 121/121 samples.
- Confirmed flush LSN: `0/1931480` to `0/1931480`; advanced: false.
- Restart LSN: `0/1931448` to `0/1931448`.
- Retained-WAL distance: start 247424, end 1874512, minimum 247424, peak 1874512 bytes; net change 1627088 bytes (2684.6 bytes/second).
- `pg_wal` directory size: 16777216 to 16777216 bytes. Checkpoint counter: 1 to 3.

## Verdict
With heartbeats disabled, the confirmed flush LSN stayed fixed while Debezium remained healthy and active. Retained-WAL distance changed by 1627088 bytes. This run reproduced the quiet-source stall.
