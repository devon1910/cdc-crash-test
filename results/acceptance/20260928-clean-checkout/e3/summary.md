# E3: connector stopped while writes continue

Question: how soon after Debezium stops do slot inactivity, health failure, and retained-WAL growth appear?

- Workload: `orders` and `noise`, each requested at 10 inserts/second for 10m0s, 4096-byte noise payload; committed 5999 orders and 5999 noise rows (actual load 10m0.005s).
- Observer: 122 samples at 5s interval plus 5s settle; largest sample gap 5.397s (coverage limit 12s).
- Slot: `cdc_crashtest_slot`. Debezium config: heartbeat disabled.
- Versions: PostgreSQL 17.11; Go go1.25.5; Compose images: cdc-crashtest-acceptance-9dc3520b0c494f728bbcae0df851eee6-receiver, quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd, postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24.
- Stop requested at 2026-09-28T12:53:13.9319401Z UTC, completed at 2026-09-28T12:53:15.5257831Z UTC (1.594s).
- Last pre-stop sample at 2026-09-28T12:53:13.9183566Z UTC: active=true, health=`UP`, flush=`0/23B9C10`, retained=542704 bytes.
- Inactive slot after completed stop: first sampled at 2026-09-28T12:53:18.9190941Z UTC (3.393s after stop completed); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=257784 bytes.
- Health not UP after completed stop: first sampled at 2026-09-28T12:53:18.9190941Z UTC (3.393s after stop completed); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=257784 bytes.
- First completed-stop sample (growth baseline): first sampled at 2026-09-28T12:53:18.9190941Z UTC (3.393s after stop completed); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=257784 bytes.
- Retained WAL above completed-stop baseline: first sampled at 2026-09-28T12:53:23.9176908Z UTC (8.392s after stop completed); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=280064 bytes.
- Confirmed flush LSN: pre-stop `0/23B9C10`, first completed-stop sample `0/23FB448`, final `0/23FB448`. Restart LSN: pre-stop `0/23769D8`, first completed-stop sample `0/23C4ED8`, final `0/23C4ED8`.
- Retained-WAL distance: start 49608, pre-stop 542704, end 1702192, peak 1702192 bytes; net growth rate after first completed-stop sample 4814.6 bytes/second.
- `pg_wal` directory size: start 33554432, end 33554432 bytes. Final health: `ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`.
- Health samples before stop request: UP 61, not UP 0; from stop request through shutdown and stopped period: UP 0, not UP 61.

Verdict: after Debezium was stopped, the first observed slot inactivity, health failure, and post-stop retained-WAL increase occurred at the times above. Each is bounded by sampling cadence; the exact instant between samples is unknown. This demonstrates source-side retention pressure, not disk exhaustion or lost events.
The runner restarts Debezium after recording the stopped-state observations. That recovery is outside this E3 measurement window.
