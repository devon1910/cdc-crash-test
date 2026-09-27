# E0 baseline: orders only

Question: with Debezium consuming continuous `orders` changes, does the slot advance while retained WAL stays small?

## Configuration and versions
- Load: `public.orders` only, 10 writes/second, requested duration 20s; 199 committed rows.
- Observer: 6 samples, 5s interval, 5s settling time, slot `cdc_crashtest_slot`.
- PostgreSQL runtime version: 17.11.
- Go runtime version: go1.25.5.
- Configured Compose images:
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-receiver`
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`

## Observations
- Sample window: 2026-09-27T17:54:11Z to 2026-09-27T17:54:36Z (UTC).
- Confirmed flush LSN: `0/1971F00` to `0/197DDC0`; advanced: true.
- Restart LSN: `0/196B408` to `0/1976158`.
- Retained-WAL distance: start 27928 bytes (0.03 MiB); end 32048 bytes (0.03 MiB); minimum 18040 bytes (0.02 MiB); peak 48832 bytes (0.05 MiB).
- Net retained-WAL growth rate: 162.38 bytes/second over the sample window.
- `pg_wal` directory size: start 16777216 bytes (16.00 MiB); end 16777216 bytes (16.00 MiB); minimum 16777216 bytes (16.00 MiB); peak 16777216 bytes (16.00 MiB).
- Active slot: 6/6 samples. Debezium health: `UP` 6/6.
- Cumulative checkpoint count: 27 to 27.

## Verdict
E0 expected behavior observed: the slot confirmed newer WAL, remained active and healthy, and peak retained-WAL distance stayed within one WAL segment (16777216 bytes (16.00 MiB)).

This one-segment bound is a local experiment rule, not a PostgreSQL safety limit. LSN distance measures WAL progress from the slot's oldest needed position; `pg_wal` size measures files on disk. Neither metric proves that every event reached the receiver.
