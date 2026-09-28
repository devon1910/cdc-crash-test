# Historical full-stack acceptance

The current heartbeat finding is the independently reset comparison in [the main report](../results/m4-comparison.md). Earlier acceptance runs are retained here as project history and should not be mixed with that comparison.

## Initial disk-fill prototype (connector stopped)

These two runs stopped Debezium before writing noise, so the health endpoint was DOWN and the slot was inactive. They established that a retained slot can exhaust a bounded PostgreSQL filesystem, but they are **not** reproductions of the healthy-but-stuck incident described in the README. The current paired experiment leaves Debezium running and supersedes these as the primary disk-fill finding.

- [First prototype run](../results/disk-fill/20260928T205504Z/summary.md), with [CSV](../results/disk-fill/20260928T205504Z/observations.csv) and [PostgreSQL log](../results/disk-fill/20260928T205504Z/postgres.log).
- [Second prototype run](../results/disk-fill/20260928T205700Z/summary.md), with [CSV](../results/disk-fill/20260928T205700Z/observations.csv) and [PostgreSQL log](../results/disk-fill/20260928T205700Z/postgres.log).

On 2026-09-28, a full local sequence exercised ordinary published writes, a quiet source, an active-heartbeat run, and a stopped connector. The earlier quiet-source and active-heartbeat scenarios were sequential and shared PostgreSQL state, so the second run inherited WAL retained during the first:

| Experiment | Historical result |
| --- | --- |
| Published-write baseline | Passed the lab heuristic; `confirmed_flush_lsn` advanced, health and slot activity were good in 122/122 samples, and peak retained-WAL distance was 541,272 bytes. |
| Quiet source, no heartbeat | 5,999 noise writes; the slot stayed active and healthy in 121 samples, but `confirmed_flush_lsn` did not move and retained-WAL distance rose from 37,776 to 1,635,256 bytes. |
| Active heartbeat, sequential follow-up | 5,999 noise writes and 59 heartbeat updates; slot LSNs advanced and retained distance ended at 375,960 bytes. Its 1,924,632-byte peak included the inherited starting backlog, so this was not a clean comparison. |
| Connector stopped during writes | The slot became inactive and health failed 3.393 seconds after Docker confirmed the stop; retained-WAL growth above the stopped-state baseline was sampled 8.392 seconds after completion. Final retained distance was 1,702,192 bytes. |

The accepted raw data and detailed summaries are in the [full-stack acceptance record](../docs/m6-acceptance.md). These historical values demonstrate why the newer heartbeat comparison resets database and offset volumes between variants.
