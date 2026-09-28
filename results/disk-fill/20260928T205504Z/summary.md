# Bounded disk-fill experiment

The run started Debezium and then stopped it, leaving its inactive logical replication slot behind. The workload wrote only to unpublished `public.noise`. PostgreSQL's data directory was a dedicated 256 MiB tmpfs; no host directory was used for database files.

- PostgreSQL 17.11; Debezium Server 3.6.3.Final.
- 23 completed batches, each eight 1 MiB rows followed by `TRUNCATE`.
- The slot was inactive and Debezium health was down before the workload.
- `confirmed_flush_lsn` stayed at `0/1931448`; `restart_lsn` stayed at `0/1931410`.
- Retained-WAL distance rose from 304,552 to 207,185,216 bytes. Allocated `pg_wal` grew from 16,777,216 to 234,881,024 bytes.
- Last successful filesystem sample: 266,387,456 of 268,435,456 bytes used, with 2,048,000 bytes available.
- The next write ended with `unexpected EOF`. [PostgreSQL's log](postgres.log) records `PANIC: No space left on device` during a checkpoint and a subsequent recovery `FATAL` when PostgreSQL could not extend a database file.

## Verdict

The bounded filesystem reached the disk-full failure mode while the inactive slot's flush position remained fixed. The final filesystem sample could not be collected after PostgreSQL exited, so the last successful measurements above are reported rather than fabricated end values. The capped tmpfs and connector-offset volume were destroyed during cleanup.
