# CDC terms used in this lab

**CDC (change data capture)** turns committed database changes into a stream of events. Here PostgreSQL writes changes, Debezium reads them through logical replication, and the Go receiver logs the resulting HTTP events.

**WAL (write-ahead log)** is PostgreSQL's ordered record of changes. PostgreSQL writes WAL so it can recover after a crash. WAL lives in segment files under `pg_wal`; a normal segment is 16 MiB, although that size is configurable at database initialization. [PostgreSQL WAL internals](https://www.postgresql.org/docs/17/wal-internals.html)

**LSN (Log Sequence Number)** is a position in WAL, written in hexadecimal as something like `0/19680C8`. Later WAL positions have larger LSNs. It is a position, not a wall-clock time or a count of orders. PostgreSQL can subtract two LSNs to get the byte distance between them. In this lab, LSN changes tell us whether Debezium's slot is making progress. [PostgreSQL WAL internals](https://www.postgresql.org/docs/17/wal-internals.html)

**Replication slot** is PostgreSQL's record of what a consumer may still need from WAL. It helps Debezium resume after interruption, but can force PostgreSQL to keep old WAL when the consumer falls behind. `active=true` only means a client is currently attached. [PostgreSQL replication slots](https://www.postgresql.org/docs/17/view-pg-replication-slots.html)

**`confirmed_flush_lsn`** is the logical slot's acknowledged position: the consumer has confirmed receiving changes through it. When this value advances during `orders` inserts, the source side of the CDC pipeline is progressing. It does not by itself prove every HTTP event arrived at the receiver. [PostgreSQL replication slots](https://www.postgresql.org/docs/17/view-pg-replication-slots.html)

**`restart_lsn`** is the oldest WAL position the slot might still need. This is the position relevant to WAL retention. It can lag behind `confirmed_flush_lsn`, so watching only acknowledgements can miss retention pressure. The lab's `retained_wal_bytes` column is the current WAL LSN minus `restart_lsn`. That byte distance is different from the size of files currently in `pg_wal`. [PostgreSQL replication slots](https://www.postgresql.org/docs/17/view-pg-replication-slots.html)

**Publication** is PostgreSQL's list of tables whose row changes are offered for logical replication. This lab publishes `public.orders` and the one-row `public.cdc_heartbeat` table used by E2. Writes to `public.noise` still generate WAL but are outside that publication.

**Checkpoint** is a point where PostgreSQL makes changed data pages durable and can recycle WAL segments that are no longer needed. A lagging slot can prevent old segments from being removed. The observer records a cumulative checkpoint counter so runs can be compared with these events. [PostgreSQL WAL configuration](https://www.postgresql.org/docs/17/wal-configuration.html)
