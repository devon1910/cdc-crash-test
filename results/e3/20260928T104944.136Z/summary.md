# E3: connector stopped while writes continue

Question: how soon after Debezium stops do slot inactivity, health failure, and retained-WAL growth appear?

- Workload: `orders` and `noise`, each requested at 10 inserts/second for 10m0s, 4096-byte noise payload; committed 5999 orders and 5999 noise rows (actual load 10m0.021s).
- Observer: 122 samples at 5s interval plus 5s settle; largest sample gap 5.471s (coverage limit 12s).
- Slot: `cdc_crashtest_slot`. Debezium config: heartbeat disabled.
- Versions: PostgreSQL 17.11; Go go1.25.5; Compose images: quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd, postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24, cdc-crashtest-receiver.
- Stop requested at 2026-09-28T10:54:44.1403691Z UTC, completed at 2026-09-28T10:54:46.5375887Z UTC (2.397s).
- Last pre-stop sample at 2026-09-28T10:54:39.1402971Z UTC: active=true, health=`UP`, flush=`0/211AED0`, retained=509600 bytes.
- Inactive slot: first sampled at 2026-09-28T10:54:49.1404391Z UTC (5s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=231280 bytes.
- Health not UP: first sampled at 2026-09-28T10:54:49.1404391Z UTC (5s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=231280 bytes.
- First completed-stop sample (growth baseline): first sampled at 2026-09-28T10:54:49.1404391Z UTC (5s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=231280 bytes.
- Retained WAL above completed-stop baseline: first sampled at 2026-09-28T10:54:54.1436137Z UTC (10.003s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=253536 bytes.
- Confirmed flush LSN: pre-stop `0/211AED0`, first completed-stop sample `0/2160FE0`, final `0/2160FE0`. Restart LSN: pre-stop `0/20DEEB0`, first completed-stop sample `0/212E188`, final `0/212E188`.
- Retained-WAL distance: start 20856, pre-stop 509600, end 1662840, peak 1662840 bytes; net growth rate after first completed-stop sample 4771.5 bytes/second.
- `pg_wal` directory size: start 33554432, end 33554432 bytes. Final health: `ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`.
- Health samples before stop: UP 60, not UP 0; after stop request: UP 1, not UP 61.

Verdict: after Debezium was stopped, the first observed slot inactivity, health failure, and post-stop retained-WAL increase occurred at the times above. Each is bounded by sampling cadence; the exact instant between samples is unknown. This demonstrates source-side retention pressure, not disk exhaustion or lost events.
The runner restarts Debezium after recording the stopped-state observations. That recovery is outside this E3 measurement window.
