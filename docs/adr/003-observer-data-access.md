# ADR-003: Observer database access and output

Status: ACCEPTED

## Context

M2 adds `cmd/observe`, which periodically records PostgreSQL replication-slot/WAL/checkpointer metrics and the Debezium health endpoint in CSV. PostgreSQL has no built-in Go `database/sql` driver, so the observer needs either a Go driver dependency or an external client process. The observer also needs a stable way to select the slot and output file.

PostgreSQL 17 exposes the needed slot fields (`active`, `restart_lsn`, `confirmed_flush_lsn`, and `wal_status`) through `pg_replication_slots`; `pg_stat_checkpointer` exposes cumulative checkpoint counters. `pg_wal_lsn_diff` provides the retained-WAL distance, while `pg_ls_waldir` can be summed for on-disk WAL size. These measurements are related but not interchangeable.

## Options

1. Use the pgx PostgreSQL driver from Go; connect directly to the local Compose-published database. Configure connection details with flags/environment variables, default the slot to `cdc_crashtest_slot`, and append samples to the requested CSV path (writing a header for a new/empty file). Query Debezium health over HTTP on each sample.
   - Pros: typed database access, no subprocess dependency, unit-testable sampling and CSV writing.
   - Cons: adds a Go module dependency; the lab's local `postgres` credentials are privileged and should remain confined to this local experiment.
2. Execute `psql` for each sample.
   - Pros: no Go database-driver dependency; familiar SQL CLI.
   - Cons: requires `psql` on the host, adds subprocess/parsing/error-handling complexity, and complicates cross-platform repeatability.
3. Implement PostgreSQL's wire protocol directly with the standard library.
   - Pros: avoids third-party dependencies.
   - Cons: disproportionate protocol and authentication complexity for this lab.

## Recommendation

Use option 1 with pgx v5.11.0 via its `database/sql` adapter. Keep the observer as a host-run Go command for M2, using the existing Compose port mapping; do not add an observer container yet. Use a configurable sample interval (default 5 seconds), slot name, DSN (with local Compose defaults), CSV path, and Debezium health URL. Append CSV rows with one UTC timestamp per sample, preserve nullable LSN/status values as empty cells, and fail clearly on database/query errors. Test CSV encoding independently of database connectivity.

For checkpoint count, record `num_timed + num_requested` from `pg_stat_checkpointer` as specified by the prompt, while noting these are cumulative counters. Record `pg_wal` directory bytes separately from LSN-distance retained bytes. The observer reports health status as returned by `/q/health`; a failed request is recorded as an unavailable/error status rather than preventing collection of database metrics.

The local lab uses PostgreSQL `trust` authentication so the observer DSN and Debezium configuration need no database password. The published host port is explicitly bound to loopback only. This trusts any client attached to this Compose network and is not appropriate for a shared or production database.

## References

- PostgreSQL 17 monitoring statistics: https://www.postgresql.org/docs/17/monitoring-stats.html
- PostgreSQL 17 replication slots: https://www.postgresql.org/docs/17/view-pg-replication-slots.html
- Debezium Server health endpoint: https://debezium.io/documentation/reference/operations/debezium-server.html
