# E0 baseline: orders only

Question: with Debezium consuming continuous `orders` changes, does the slot advance while retained WAL stays small?

## Configuration and versions
- Load: `public.orders` only, 10 writes/second, requested duration 2m0s; 1199 committed rows.
- Actual load wall time: 2m0.026s; observed average 9.99 committed rows/second.
- Observer: 26 samples, 5s interval, 5s settling time, slot `cdc_crashtest_slot`.
- PostgreSQL runtime version: 17.11.
- Go runtime version: go1.25.5.
- Configured Compose images:
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-receiver`
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`

## Observations
- Sample window: 2026-09-27T18:23:06Z to 2026-09-27T18:25:13Z (UTC).
- Confirmed flush LSN: `0/1A85618` to `0/1AD7890`; advanced: true.
- Restart LSN: `0/1A72250` to `0/1AAEDC8`.
- Retained-WAL distance: start 84000 bytes (0.08 MiB); end 166856 bytes (0.16 MiB); minimum 16536 bytes (0.02 MiB); peak 320920 bytes (0.31 MiB).
- Net retained-WAL growth rate: 652.76 bytes/second over the sample window.
- `pg_wal` directory size: start 16777216 bytes (16.00 MiB); end 16777216 bytes (16.00 MiB); minimum 16777216 bytes (16.00 MiB); peak 16777216 bytes (16.00 MiB).
- Active slot: 26/26 samples. Debezium health: `UP` 26/26.
- Cumulative checkpoint count: 32 to 33.
- Largest gap between samples: 6.906s; coverage rule allows at most 12s.

## Verdict
E0 expected behavior observed: the slot confirmed newer WAL, remained active and healthy, and peak retained-WAL distance stayed within one WAL segment (16777216 bytes (16.00 MiB)).

This one-segment bound is a local experiment rule, not a PostgreSQL safety limit. LSN distance measures WAL progress from the slot's oldest needed position; `pg_wal` size measures files on disk. Neither metric proves that every event reached the receiver.
