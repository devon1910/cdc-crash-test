# M4 clean comparison: heartbeat variants

Three independent full-duration runs were measured on 2026-09-28. Before **each** scenario, the isolated Compose project's PostgreSQL and Debezium volumes were removed and recreated. This resets the database, replication slot, and connector offsets; the original local lab volumes were not touched. Each run used 5,999 committed 4 KiB inserts into unpublished `public.noise` at 10 inserts/second, with no `orders` writes. Sampling was every 5 seconds for 10 minutes, followed by a 5-second settle. PostgreSQL was 17.11, Debezium Server was 3.6.3.Final, and Go was 1.25.5.

| Measurement | E1: no heartbeat | E2 timer-only | E2 active action query |
| --- | ---: | ---: | ---: |
| Samples; slot active; Debezium `UP` | 121; all; all | 121; all; all | 122; all; all |
| Heartbeat table updates | 0 | 0 | 59 |
| `confirmed_flush_lsn` | `0/1931480` → `0/1931480` (fixed) | `0/19322F0` → `0/19322F0` (fixed) | `0/19322F0` → `0/1AEC840` (advanced) |
| Retained-WAL distance, start → end | 247,424 → 1,874,512 B | 96 → 1,918,712 B | 368 → 327,320 B |
| Retained-WAL net change; peak | +1,627,088 B; 1,874,512 B | +1,918,616 B; 1,918,712 B | +326,952 B; 598,888 B |
| `pg_wal` allocated size, start → end | 16 → 16 MiB | 16 → 16 MiB | 16 → 16 MiB |

## Verdict

In this setup, `heartbeat.interval.ms=10000` **by itself was not enough**: the timer-only run matched the no-heartbeat run in the important respects. Both had an active, healthy connector, a stationary confirmed-flush LSN, and growing retained-WAL distance. The action query that changed the published `cdc_heartbeat` row produced 59 heartbeat updates, advanced the confirmed-flush LSN, and kept retained-WAL distance substantially lower and bounded during this measured window.

The active-heartbeat result supports this configuration for this lab; it is not a production guarantee. The results do not prove every event reached the receiver. Retained-WAL distance is an LSN-distance indicator, not filesystem usage; the physical `pg_wal` directory stayed at 16 MiB in all three runs. The active case had a 598,888-byte peak, so retention was not zero.

Because each case had its own fresh baseline, compare the changes and trends rather than the absolute LSN values or initial WAL distances. This avoids the inherited E1 backlog present in the earlier sequential comparison.

Raw evidence and per-run details:

- [E1 summary](m4-clean-comparison/e1/summary.md) and [CSV](m4-clean-comparison/e1/observations.csv)
- [Timer-only summary](m4-clean-comparison/e2-timer/summary.md) and [CSV](m4-clean-comparison/e2-timer/observations.csv)
- [Active action-query summary](m4-clean-comparison/e2/summary.md) and [CSV](m4-clean-comparison/e2/observations.csv)
