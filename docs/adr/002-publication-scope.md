# ADR-002: PostgreSQL publication scope

Status: PROPOSED

## Context

The lab captures `public.orders` and generates WAL through `public.noise`. With the PostgreSQL `pgoutput` plug-in, the publication's table set determines which row changes can reach Debezium. Debezium's `table.include.list` separately filters emitted records. Its documented default publication mode can include all tables, even when an include list is configured: https://debezium.io/documentation/reference/3.5/connectors/postgresql.html

## Options

1. Publish only `public.orders`; use `table.include.list=public.orders`. This makes `noise` absent from the logical change stream and gives E1 a clear meaning. It requires an explicit or filtered publication and may make interval-only heartbeats insufficient to advance the slot.
2. Publish all tables; use `table.include.list=public.orders`. This is closer to Debezium's default publication behavior and simpler to configure. Debezium may still process `noise` changes before filtering them, which could change the result of E1.
3. Publish `orders` and `noise`, then filter `noise` from emitted records. This tests filtering behavior explicitly, but the source is no longer quiet at the publication boundary.

## Recommendation

Publish only `public.orders` and use `table.include.list=public.orders`. Put both tables in the same PostgreSQL database. In M1, inspect the publication membership as well as the active slot. E1 and E2 should report their observed LSN behavior without assuming that interval-only heartbeats will solve the quiet-publication case. Any later heartbeat-action table or publication change needs a separate proposed decision before M4.
