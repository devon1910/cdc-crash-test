# Disk-fill repeatability check

Two independently reset pairs used the same isolated 256 MiB PostgreSQL tmpfs, PostgreSQL 17.11, Debezium Server 3.6.3.Final, 30-batch limit, eight 1 MiB noise rows per batch followed by `TRUNCATE`, and 10-second pacing. Debezium ran in both scenarios; only the action-query scenario updated a published heartbeat table every 10 seconds. Each case had a fresh connector-offset volume, which was removed with the tmpfs after the run.

| Pair | Scenario | Completed batches | Outcome | Peak retained-WAL distance | Peak filesystem use |
| --- | --- | ---: | --- | ---: | ---: |
| [First](disk-fill/20260928T214209Z-no-heartbeat/summary.md) | No heartbeat | 23/30 | Batch 24 hit disk-full SQLSTATE 53100 | 207,208,008 bytes | 268,435,456 / 268,435,456 bytes |
| [First](disk-fill/20260928T214702Z-action-query/summary.md) | Action query | 30/30 | No disk-full failure within budget | 99,140,864 bytes | 182,669,312 / 268,435,456 bytes |
| [Repeat](disk-fill/20260929T105511Z-no-heartbeat/summary.md) | No heartbeat | 23/30 | Batch 24 hit disk-full SQLSTATE 53100 | 207,323,328 bytes | 268,283,904 / 268,435,456 bytes |
| [Repeat](disk-fill/20260929T110028Z-action-query/summary.md) | Action query | 30/30 | No disk-full failure within budget | 108,133,424 bytes | 182,706,176 / 268,435,456 bytes |

In both no-heartbeat runs, the slot's confirmed flush LSN did not advance, retained-WAL distance grew, and Debezium health was `UP` in every recorded sample. PostgreSQL logged disk-full `PANIC` in both. In both action-query runs, the slot advanced, health remained `UP`, and all 30 batches finished. The heartbeat did **not** eliminate retained WAL: its measured peak was higher in the repeat (108 MB versus 99 MB), while the filesystem stayed below the cap.

Timing matters for the health claim. In the first no-heartbeat run, the final CSV sample began about 84 ms *after* PostgreSQL logged `PANIC` and still read Debezium health `UP`. In the repeat, the final CSV sample began about 1.5 seconds *before* the logged `PANIC`; it cannot corroborate the post-crash reading. Both samples' PostgreSQL metrics queries failed. The raw CSVs are [first no-heartbeat](disk-fill/20260928T214209Z-no-heartbeat/observations.csv), [first action-query](disk-fill/20260928T214702Z-action-query/observations.csv), [repeat no-heartbeat](disk-fill/20260929T105511Z-no-heartbeat/observations.csv), and [repeat action-query](disk-fill/20260929T110028Z-action-query/observations.csv); each linked run summary also links its PostgreSQL log.

The outcome reproduced in **two of two local paired trials** under this fixed workload and cap. That is stronger than a single demonstration, but does not justify an unqualified “every time” claim for other loads, heartbeat intervals, or environments.
