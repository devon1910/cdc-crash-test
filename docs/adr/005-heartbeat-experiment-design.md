# ADR-005: E2 heartbeat design for the quiet-publication test

Status: ACCEPTED — user chose option 2 (active heartbeat with a small table)

## Context

E1 and E2 generate writes to `public.noise` but no application writes to `public.orders`. The current PostgreSQL publication contains only `orders`, so `noise` cannot supply row changes to Debezium. E1 keeps heartbeats off; E2 should test whether a heartbeat prevents replication-slot WAL retention under the same noise workload.

Debezium 3.6 distinguishes `heartbeat.interval.ms` (periodic heartbeat records) from `heartbeat.action.query` (SQL run at each heartbeat). Its PostgreSQL documentation says an action-query heartbeat table must be in the publication so its changes can be detected and processed. Thus, adding an action query also changes the database schema and publication. Sources: [Debezium 3.6 PostgreSQL connector](https://debezium.io/documentation/reference/3.6/connectors/postgresql.html), sections “heartbeat.interval.ms” and “heartbeat.action.query”.

## Options

1. **Timer-only E2.** Set `heartbeat.interval.ms` to 10 seconds and leave the orders-only publication unchanged. This changes only one Debezium setting and tests whether periodic heartbeat records alone are sufficient. With no published row changes, it may not advance `confirmed_flush_lsn`; a negative result would not show that action-query heartbeats fail.
2. **Published action-query E2 (recommended).** Add one small `public.cdc_heartbeat` table to the publication and capture filter for both E1 and E2, but never change it during E1. In E2 set `heartbeat.interval.ms=10000` and `heartbeat.action.query` to update that table's single row with `clock_timestamp()`. Keep `public.noise` out of the publication in both runs. This gives E2 a genuine published change to acknowledge while preserving the same publication and filter across E1/E2. It adds a synthetic captured table and grants Debezium write permission, so this tests an active heartbeat strategy, not the timer alone.
3. **Run both E2 variants.** First try timer-only, then action-query with the table. This isolates the timer's effect, but needs a third run and two E2 result sets; the publication change still needs careful labeling.

## Recommendation and controls

Choose option 2 for M4. Use the same noise write rate, duration, sample interval, slot, publication membership, and Debezium capture filter for E1 and E2. Change only heartbeat settings between runs. Record the committed noise count, slot activity, `confirmed_flush_lsn`, retained WAL distance, health, sample gaps, and heartbeat-table update count. Report observed differences rather than promising a particular outcome. Do not count heartbeat updates as application `orders` writes.

If approved, M4 may add the heartbeat table and grants, extend the publication and capture filter in both runs, implement `make e1` and `make e2`, and write results and a comparison report. Existing databases will need an explicit, non-destructive migration because init SQL only runs on a fresh PostgreSQL volume.

## Decision needed

User approved option 2 before M4 implementation. Timer-only behavior remains untested by this decision.
