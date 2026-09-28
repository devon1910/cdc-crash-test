# Bounded disk-fill experiment: action-query

Debezium remained running throughout the workload. The workload wrote only to unpublished `public.noise`; PostgreSQL used a dedicated 256 MiB tmpfs, not a host data directory. Scenario: **action-query**.

- PostgreSQL: 17.11. Debezium Server: 3.6.3.Final.
- Workload: 30 completed batches out of a 30-batch budget, each 8 rows of 1 MiB text, followed by `TRUNCATE`.
- Debezium health was `UP` at start; health was UP and the slot active in every sample: true.
- Heartbeat: 10-second interval plus action query on published `public.cdc_heartbeat`; update count: 1 to 34; confirmed flush LSN: `0/1931480` at start to `0/F8E6840` at last successful sample.
- Restart LSN: `0/1931448` to `0/D694B18`.
- Retained-WAL distance: 368 at start to 72000448 bytes at the last successful sample. `pg_wal` allocated size: 16777216 to 100663296 bytes.
- Peak retained-WAL distance: 99140864 bytes. Peak PostgreSQL data filesystem usage: 182669312 / 268435456 bytes.
- PostgreSQL data filesystem: 48021504 / 268435456 bytes used at start; last successful sample 132354048 / 268435456 bytes used with 136081408 bytes available.
- Terminal write result: safety batch limit reached. Container state at the final metrics probe: `running`. See the [PostgreSQL log](postgres.log).

## Verdict
PostgreSQL did not run out of space within the same 30-batch budget. The heartbeat action query advanced the slot while WAL remained bounded. The bounded tmpfs and connector-offset volume were destroyed during cleanup.
