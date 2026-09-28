# Bounded disk-fill experiment: no-heartbeat

Debezium remained running throughout the workload. The workload wrote only to unpublished `public.noise`; PostgreSQL used a dedicated 256 MiB tmpfs, not a host data directory. Scenario: **no-heartbeat**.

- PostgreSQL: 17.11. Debezium Server: 3.6.3.Final.
- Workload: 23 completed batches out of a 30-batch budget, each 8 rows of 1 MiB text, followed by `TRUNCATE`.
- Debezium health was `UP` at start; health was UP and the slot active in every sample before PostgreSQL crashed: true.
- Heartbeat: disabled (`heartbeat.interval.ms=0`); confirmed flush LSN: `0/1931448` at start to `0/1931448` at last successful sample.
- Restart LSN: `0/1931410` to `0/1931410`.
- Retained-WAL distance: 96 at start to 207208008 bytes at the last successful sample. `pg_wal` allocated size: 16777216 to 218103808 bytes.
- Peak retained-WAL distance: 207208008 bytes. Peak PostgreSQL data filesystem usage: 268435456 / 268435456 bytes.
- PostgreSQL data filesystem: 48021504 / 268435456 bytes used at start; last successful sample 249835520 / 268435456 bytes used with 18599936 bytes available.
- Terminal write result: PostgreSQL logged `No space left on device` and stopped during checkpoint/recovery; the client received PostgreSQL SQLSTATE 53100: could not write to file "pg_wal/xlogtemp.1152": No space left on device. Container state at the final metrics probe: `running`. See the [PostgreSQL log](postgres.log).
- Final metrics query: unavailable after PostgreSQL stopped (`failed to connect to `user=postgres database=cdc_lab`: 127.0.0.1:55434 (127.0.0.1): server error: FATAL: could not write init file: No space left on device (SQLSTATE 53100)`). Last successful metrics are shown above.

## Verdict
PostgreSQL ran out of space after 23 batches. Debezium health was UP in every sample until PostgreSQL crashed. The confirmed flush position stopped advancing while retained WAL grew. The bounded tmpfs and connector-offset volume were destroyed during cleanup.
