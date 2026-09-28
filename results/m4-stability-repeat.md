# Heartbeat comparison: repeatability check

The three 10-minute scenarios were repeated with a fresh PostgreSQL database, replication slot, and Debezium offset volume before each run. Workload and software versions matched the original clean comparison: 5,999 4 KiB unpublished `noise` inserts at 10/second; PostgreSQL 17.11; Debezium Server 3.6.3.Final; Go 1.25.5. Each included 5-second sampling and settling settings.

| Scenario | First clean run: flush LSN / retained-WAL change / peak | Repeat: flush LSN / retained-WAL change / peak | Finding repeated? |
| --- | --- | --- | --- |
| No heartbeat | Fixed / +1,627,088 B / 1,874,512 B | Fixed / +1,899,720 B / 1,899,872 B | Yes: active and healthy, but no flush progress and WAL distance grew. |
| Timer-only (`heartbeat.interval.ms=10000`) | Fixed / +1,918,616 B / 1,918,712 B | Fixed / +1,903,184 B / 1,903,336 B | Yes: timer-only did not advance the slot; WAL distance grew. |
| Published-table action query | Advanced / +326,952 B / 598,888 B | Advanced / +353,792 B / 375,088 B | Yes: flush progress continued and retained distance stayed far below the other cases. |

All three repeats had Debezium health `UP` in every sample and an active slot throughout. The repeats confirm the original conclusion: in this workload, timer-only heartbeats were not sufficient, while the published-table action query advanced the confirmed flush LSN and kept retained-WAL distance much lower. The precise byte values vary by run; the behavior classification did not.

The first attempted E1 repeat had a 9m38s observation gap and was marked inconclusive by the runner, so it is excluded from the table. E1 was rerun with 122 samples and a largest gap of 5.356s; the valid rerun is the one linked below.

Raw evidence for the valid repeats:

- [No-heartbeat summary](m4-stability-repeat/e1/summary.md), [CSV](m4-stability-repeat/e1/observations.csv)
- [Timer-only summary](m4-stability-repeat/e2-timer/summary.md), [CSV](m4-stability-repeat/e2-timer/observations.csv)
- [Action-query summary](m4-stability-repeat/e2/summary.md), [CSV](m4-stability-repeat/e2/observations.csv)
