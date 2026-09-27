# M2: Observer

`cmd/observe` samples the configured PostgreSQL logical replication slot and Debezium health endpoint, appending one sample immediately and then at the configured interval. It runs on the host and uses the Compose-published PostgreSQL port.

## Run

Start the existing M1 stack, then run the observer from the repository root:

```powershell
go run ./cmd/observe -interval=5s -csv=results/m2-observations.csv
```

The defaults are slot `cdc_crashtest_slot`, local DSN `postgres://postgres@127.0.0.1:55432/cdc_lab?sslmode=disable`, CSV path `results/observations.csv`, and health URL `http://localhost:8080/q/health`. The local Compose lab uses passwordless trust authentication and binds PostgreSQL's published port to loopback; any container attached to the Compose network is trusted too, so keep this configuration local-only. The host-side port is `55432` to avoid conflicting with a PostgreSQL server already listening on the usual `5432`; Postgres still listens on `5432` inside the Compose network. Override the defaults with `-slot`, `-dsn`, `-csv`, and `-health-url`; `DATABASE_URL` can supply the DSN. Stop with Ctrl+C. Rows append to an existing CSV; the header is written only for a new or empty file.

## CSV columns

| Column | Meaning |
|---|---|
| `timestamp` | UTC time when the sample began. |
| `slot_name` | Replication slot sampled. |
| `active` | Whether a replication client is currently connected to the slot. |
| `restart_lsn` | Oldest WAL position the slot may still require. |
| `confirmed_flush_lsn` | WAL position the logical consumer has confirmed. |
| `wal_status` | PostgreSQL's slot WAL availability status. |
| `retained_wal_bytes` | `pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)`: distance in WAL bytes, not filesystem usage. Blank if `restart_lsn` is NULL. |
| `pg_wal_bytes` | Sum of the current files reported by `pg_ls_waldir()`. |
| `debezium_health_status` | Aggregate status returned by Debezium `/q/health`; HTTP/network/response failures are recorded as `HTTP_*` or `ERROR: ...`. |
| `checkpoint_count` | Cumulative `num_timed + num_requested` from `pg_stat_checkpointer`; it is a counter, not a per-interval delta. |

The two WAL columns intentionally report different things: LSN distance estimates slot-retained WAL progress, while directory size measures current files in `pg_wal`. PostgreSQL may retain/recycle whole segment files, so these values need not match. See the [PostgreSQL 17 replication-slot view](https://www.postgresql.org/docs/17/view-pg-replication-slots.html) and [monitoring statistics](https://www.postgresql.org/docs/17/monitoring-stats.html).

The default local DSN uses the lab's privileged `postgres` account because the observer reads server-wide slot, checkpointer, and WAL-directory information. Trust authentication means a client that can reach the database does not need to know a password; the loopback port binding limits host access to the local machine but does not isolate clients on the Compose network.
