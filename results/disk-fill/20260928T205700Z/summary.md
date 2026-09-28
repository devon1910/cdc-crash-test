# Bounded disk-fill experiment

This run started Debezium, then stopped the connector while leaving its logical replication slot behind. The workload wrote only to the unpublished `public.noise` table. PostgreSQL's data directory was a dedicated 256 MiB tmpfs; no host directory was used for database files.

- PostgreSQL: 17.11. Debezium Server: 3.6.3.Final.
- Workload batches: 23 completed batches, each 8 rows of 1 MiB text, followed by `TRUNCATE` so the table could reuse its space.
- Consumer state: Debezium stopped; the slot remained inactive in every successful slot sample; health after stop: `DOWN` (expected; connector stopped).
- Confirmed flush LSN: `0/1931448` at start to `0/1931448` at the last successful sample; restart LSN: `0/1931410` to `0/1931410`.
- Retained-WAL distance: 96 at start to 206932784 bytes at the last successful sample. `pg_wal` allocated size: 16777216 to 218103808 bytes.
- PostgreSQL data filesystem: 48021504 / 268435456 bytes used at start; last successful sample 249348096 / 268435456 bytes used with 19087360 bytes available.
- Terminal write result: PostgreSQL logged `No space left on device` and stopped during checkpoint/recovery; the client received PostgreSQL SQLSTATE 53100: could not write to file "pg_wal/xlogtemp.329": No space left on device. Container state at the final metrics probe: `running`. See the [PostgreSQL log](postgres.log).
- Final metrics query: unavailable after PostgreSQL stopped (`failed to connect to `user=postgres database=cdc_lab`: 127.0.0.1:55434 (127.0.0.1): dial error: dial tcp 127.0.0.1:55434: connectex: No connection could be made because the target machine actively refused it.`). Last successful metrics are shown above.

## Verdict
The bounded filesystem reached a PostgreSQL disk-full write failure while the inactive slot's confirmed flush position remained behind current WAL. This demonstrates the failure mechanism without filling the host filesystem. The database filesystem and connector-offset volume were destroyed during cleanup.
