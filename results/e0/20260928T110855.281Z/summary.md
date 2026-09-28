# E0 baseline: orders only

Question: with Debezium consuming continuous `orders` changes, does the slot advance while retained WAL stays small?

## Configuration and versions
- Load: `public.orders` only, 10 writes/second, requested duration 1m30s; 899 committed rows.
- Actual load wall time: 1m30.017s; observed average 9.99 committed rows/second.
- Observer: 20 samples, 5s interval, 5s settling time, slot `cdc_crashtest_slot`.
- PostgreSQL runtime version: 17.11.
- Go runtime version: go1.25.5.
- Configured Compose images:
  - `quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd`
  - `postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24`
  - `cdc-crashtest-receiver`

## Observations
- Sample window: 2026-09-28T11:08:54Z to 2026-09-28T11:10:30Z (UTC).
- Confirmed flush LSN: `0/22C3E40` to `0/22F6188`; advanced: true.
- Restart LSN: `0/216F248` to `0/22E60C0`.
- Retained-WAL distance: start 1396784 bytes (1.33 MiB); end 65936 bytes (0.06 MiB); minimum 20008 bytes (0.02 MiB); peak 1396784 bytes (1.33 MiB).
- Net retained-WAL growth rate: -13887.62 bytes/second over the sample window.
- `pg_wal` directory size: start 33554432 bytes (32.00 MiB); end 33554432 bytes (32.00 MiB); minimum 33554432 bytes (32.00 MiB); peak 33554432 bytes (32.00 MiB).
- Active slot: 20/20 samples. Debezium health: `UP` 20/20.
- Cumulative checkpoint count: 95 to 96.
- Largest gap between samples: 5.812s; coverage rule allows at most 12s.

## Verdict
E0 expected behavior observed: the slot confirmed newer WAL, remained active and healthy, and peak retained-WAL distance stayed within one WAL segment (16777216 bytes (16.00 MiB)).

This one-segment bound is a local experiment rule, not a PostgreSQL safety limit. LSN distance measures WAL progress from the slot's oldest needed position; `pg_wal` size measures files on disk. Neither metric proves that every event reached the receiver.
