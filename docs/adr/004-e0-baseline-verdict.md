# ADR-004: E0 baseline verdict

Status: PROPOSED

## Context

M3 asks E0 to show that the slot advances and retained WAL "stays flat." The observer records a byte distance from the current WAL position to the slot's `restart_lsn`; that distance may fluctuate between samples. A reproducible summary needs to show the raw range and define what counts as a small baseline.

## Options

1. Call E0's expected behavior observed when `confirmed_flush_lsn` advances, the slot is active and Debezium health is `UP` in every sample, the peak retained-WAL distance is no more than one PostgreSQL WAL segment (normally 16 MiB), and no sampling gap exceeds two configured intervals plus two seconds. Report the start, end, minimum, maximum, growth rate, actual segment size, and largest sampling gap.
   - Pros: a concrete, easy-to-check bound tied to PostgreSQL's WAL storage unit; allows small timing fluctuations.
   - Cons: a lab heuristic, not a production safety threshold; a pre-existing backlog or a slow observer can fail the run before the new load matters.
2. Require end retained-WAL distance to be no greater than start.
   - Pros: simple zero-net-growth rule.
   - Cons: one sample's position in a checkpoint or flush cycle can turn a healthy run into a failure; it ignores peaks.
3. Report numbers without an automated pass/fail verdict.
   - Pros: avoids an arbitrary bound.
   - Cons: fails the experiment's requirement for a plain, reproducible verdict and makes later comparisons harder.

## Recommendation

Use option 1 for E0, and label it as a lab acceptance rule. Keep the raw measurements in the CSV and summary so the verdict can be challenged. A passing E0 does not prove that every event arrived at the receiver or that retention will remain small under other workloads.

## References

- PostgreSQL 17 WAL internals and normal 16 MiB segment size: https://www.postgresql.org/docs/17/wal-internals.html
- PostgreSQL 17 replication-slot LSN fields: https://www.postgresql.org/docs/17/view-pg-replication-slots.html
