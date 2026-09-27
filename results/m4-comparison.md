# M4 comparison: E1 versus E2

Both full runs used the same 10-minute noise-only workload at 10 inserts/second, with 5-second slot samples and a 5-second settling sample. Each committed 5,999 `noise` rows and zero `orders` rows from the runner. The publication contained `orders` and `cdc_heartbeat`, not `noise`, in both runs. The connector was recreated before each run, preserving PostgreSQL data, the replication slot, and Debezium offsets; only the heartbeat settings differed.

| Measurement | E1: heartbeat off | E2: active heartbeat |
| --- | ---: | ---: |
| Samples / largest gap | 122 / 5.028 s | 122 / 5.021 s |
| Slot active, Debezium UP | All samples | All samples |
| Heartbeat table updates | 0 | 59 |
| `confirmed_flush_lsn` | Fixed at `0/1C7DCB8` | `0/1E088A0` → `0/1F76470` |
| `restart_lsn` | Fixed at `0/1C7CF90` | `0/1E087D8` → `0/1F4DA48` |
| Retained-WAL LSN distance, start → end | 3,960 → 1,609,248 bytes | 792 → 264,768 bytes |
| Peak retained-WAL LSN distance | 1,609,248 bytes | 377,400 bytes |
| `pg_wal` allocated on disk, start → end | 32 → 32 MiB | 32 → 32 MiB |

**Finding:** This local run supports the active-heartbeat strategy. With heartbeats off, an otherwise healthy Debezium connection did not confirm new WAL positions, the slot's restart position stayed fixed, and retained-WAL distance rose throughout the 10-minute workload. With the published heartbeat action query, both positions advanced and retained-WAL distance stayed much lower: the E2 peak was about 23% of E1's final distance. E2 distance fluctuated rather than remaining zero, so this is bounded retention during the measured window, not proof that WAL can never accumulate.

The runs were sequential, so their absolute starting LSNs differ. The E2 container restart also let the slot start near current WAL; the meaningful comparison is how each run evolved after its own start. The physical `pg_wal` directory remained 32 MiB in both runs because PostgreSQL allocates/recycles segment files; the LSN distance is the more informative slot-pressure measure here. We did not test timer-only heartbeats, other write rates, long-term behavior, failures, or complete end-to-end delivery. This is a lab result, not a production guarantee.

Raw evidence: [E1 observations](e1/20260927T211132.305Z/observations.csv), [E1 summary](e1/20260927T211132.305Z/summary.md), [E2 observations](e2/20260927T212252.042Z/observations.csv), [E2 summary](e2/20260927T212252.042Z/summary.md).
