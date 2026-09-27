# E0 baseline: orders only

Question: with Debezium consuming continuous `orders` changes, does the slot advance while retained WAL stays small?

## Configuration and versions
- Load: `public.orders` only, 10 writes/second, requested duration 10m0s; 4625 committed rows.
- Observer: 94 samples, 5s interval, 5s settling time, slot `cdc_crashtest_slot`.
- PostgreSQL runtime version: 17.11.
- Go runtime version: go1.25.5.
- Configured Compose images:
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-receiver`
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`

## Observations
- Sample window: 2026-09-27T17:56:06Z to 2026-09-27T18:08:20Z (UTC).
- Confirmed flush LSN: `0/197DDC0` to `0/1A85618`; advanced: true.
- Restart LSN: `0/1976158` to `0/1A72250`.
- Retained-WAL distance: start 47400 bytes (0.05 MiB); end 78992 bytes (0.08 MiB); minimum 32728 bytes (0.03 MiB); peak 300536 bytes (0.29 MiB).
- Net retained-WAL growth rate: 43.09 bytes/second over the sample window.
- `pg_wal` directory size: start 16777216 bytes (16.00 MiB); end 16777216 bytes (16.00 MiB); minimum 16777216 bytes (16.00 MiB); peak 16777216 bytes (16.00 MiB).
- Active slot: 94/94 samples. Debezium health: `UP` 94/94.
- Cumulative checkpoint count: 28 to 29.
- Largest sample gap: 272.72 seconds, from 2026-09-27T18:03:47Z to 2026-09-27T18:08:20Z.

## Verdict
E0 is inconclusive: the 272.72-second sampling gap means slot activity, Debezium health, and retained-WAL behavior during that interval are unknown. The recorded samples show an advancing slot and a peak retained-WAL distance of 0.29 MiB, but they do not establish continuous baseline behavior for the whole run. Repeat E0 before continuing to E1.

This one-segment bound is a local experiment rule, not a PostgreSQL safety limit. LSN distance measures WAL progress from the slot's oldest needed position; `pg_wal` size measures files on disk. Neither metric proves that every event reached the receiver.
