# Defend it questions and answers

This file records milestone ownership questions and concise answers for later study.

## M0 — Plan

### 1. Why does PostgreSQL publication membership matter when Debezium also has a table include list?

The publication controls which table changes PostgreSQL makes available through `pgoutput`. Debezium's table include list is a separate capture filter. Publishing only `public.orders` keeps `public.noise` outside the logical change stream and makes E1's setup easier to interpret. The experiment must still measure slot positions to determine whether the quiet publication causes the slot to lag; it should not assume the outcome.

### 2. Why report both LSN distance and the size of `pg_wal`?

LSN distance measures how far the current WAL position is from the slot's `restart_lsn`. It is a measure of WAL progress/retention distance, not a direct disk-usage measurement. PostgreSQL retains and recycles whole WAL segment files at checkpoints, and other settings can also affect directory size. Reporting both shows slot pressure and actual on-disk WAL use.

### 3. What does an HTTP receiver log prove about CDC delivery, and what does it leave unproven?

For a known insert, a receiver log entry proves that an event reached the receiver and was written to its container log, with a receive timestamp. It does not prove that every event arrived, that delivery was exactly once, or that the receiver can recover the event after a restart. Retries can also produce duplicate deliveries.

## M1 — Stack up

### 4. Why create a PostgreSQL publication containing only `orders`?

E1 is meant to generate WAL with writes to `noise` while Debezium captures `orders`. A publication containing only `orders` makes that boundary explicit: `noise` writes are generated in PostgreSQL but are not published as row changes. This reduces ambiguity about whether Debezium processed `noise` changes before filtering them. The observer will establish the actual slot behavior from recorded LSNs.

### 5. Why persist Debezium offsets in a named volume?

Debezium uses offsets to remember how far it has processed the source log. A named volume lets that state survive container replacement, so restarting the service can resume from its previous position instead of behaving like a fresh deployment. `docker compose down` preserves named volumes; `docker compose down --volumes` removes them and resets the lab state.

### 6. What do the active slot and receiver log prove, and what do they leave unproven?

An active replication slot shows that a replication client is currently attached to the slot. A receiver log entry for the test insert shows that a corresponding event reached the HTTP receiver. Together, these verify the basic M1 path from PostgreSQL through Debezium to the receiver. They do not establish complete, durable, exactly-once delivery or prove that no events could be missed during a failure.

## M2 — Observer

### 7. Why record both retained-WAL LSN distance and `pg_wal` directory size?

The LSN difference is the byte distance from the slot's `restart_lsn` to the current WAL position; it is a useful slot-retention indicator, but not a direct measure of files consuming disk. PostgreSQL retains and recycles WAL segment files at checkpoints, so actual directory size can move differently. Recording both distinguishes logical retention pressure from current filesystem use.

### 8. What does an advancing `confirmed_flush_lsn` prove, and what does it not prove?

It shows that the logical replication consumer has confirmed processing through a later WAL position. During committed `orders` inserts, it is evidence that the Debezium source is making progress. By itself it does not prove each corresponding HTTP request arrived, that delivery is durable, or that delivery is exactly once; those require separate sink-side evidence and stronger guarantees.

### 9. Why keep sampling PostgreSQL when the Debezium health request fails?

The experiment is intended to correlate database-side slot/WAL behavior with Debezium's reported health. Recording a health error alongside the PostgreSQL sample preserves that comparison; aborting the sample would discard precisely the observations that can matter during an unhealthy interval. Database query errors still stop the observer because the core measurement would be incomplete.

### 10. What is the risk of PostgreSQL `trust` authentication, and how is it constrained here?

With `trust`, PostgreSQL does not verify a password for connections matching the `pg_hba.conf` rules, so any client that can reach the database over an allowed path can claim a database role, including the privileged `postgres` role. The Compose host port is therefore bound to `127.0.0.1`, limiting host access to this machine, while clients attached to the lab's Docker network are also trusted. This is acceptable only for this isolated local lab; do not reuse the configuration for shared, LAN-exposed, or production databases.

## M3 — Load generator and E0

### 11. Why does the load generator make separate committed inserts rather than one long transaction?

Logical decoding exposes committed changes. Separate inserts executed outside an explicit transaction commit individually, giving Debezium regular opportunities to process new `orders` changes. One long transaction would delay visible CDC progress until it commits and make the E0 time series hard to interpret.

### 12. Why check both `confirmed_flush_lsn` and `restart_lsn` during E0?

`confirmed_flush_lsn` shows how far the logical consumer has acknowledged data, so an increase confirms source-side progress. `restart_lsn` is the oldest WAL location that might still be needed by the slot; its distance from current WAL determines retention pressure. The positions can move differently, so one cannot substitute for the other.

### 13. Why use one WAL segment as E0's small-retention rule, and what does a pass not prove?

A segment-sized bound is a simple, visible local acceptance rule tied to PostgreSQL's WAL storage unit. E0 also reports the raw measurements, so this heuristic can be challenged. The runner requires regular samples, because a long gap could hide retention or health changes. Passing means that the slot advanced and its sampled LSN-distance retention stayed within that bound during a sufficiently observed run. It does not prove durable or exactly-once receiver delivery, nor that the bound is safe for production.

## M4 — Quiet publication and active heartbeat

### 14. Why is the heartbeat table in the publication for both E1 and E2?

Keeping publication membership and Debezium's capture filter identical avoids making the publication change itself an E1/E2 difference. E1 leaves the row untouched. E2's action query updates it every 10 seconds, creating a published change Debezium can process. The `noise` table stays outside the publication in both runs.

### 15. Why use an action query rather than only `heartbeat.interval.ms`?

With no `orders` writes and `noise` unpublished, a timer-only heartbeat may not give the connector a new published WAL change to acknowledge. The action query creates one deliberately. This tests an active heartbeat strategy, not whether the timer alone is sufficient; that is a separate possible experiment.

### 16. If `confirmed_flush_lsn` moves, has the WAL-retention problem been solved?

Not necessarily. `restart_lsn` marks the oldest WAL still potentially needed by the slot. It can remain stationary while `confirmed_flush_lsn` advances. We therefore compare the distance from current WAL to `restart_lsn` across E1 and E2, as well as both LSNs. Short runs in particular may show flush progress without evidence that retained WAL has stopped growing.
