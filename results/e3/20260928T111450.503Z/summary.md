# E3: connector stopped while writes continue

Question: how soon after Debezium stops do slot inactivity, health failure, and retained-WAL growth appear?

- Workload: `orders` and `noise`, each requested at 10 inserts/second for 1m30s, 4096-byte noise payload; committed 899 orders and 899 noise rows (actual load 1m30.004s).
- Observer: 20 samples at 5s interval plus 5s settle; largest sample gap 5.618s (coverage limit 12s).
- Slot: `cdc_crashtest_slot`. Debezium config: heartbeat disabled.
- Versions: PostgreSQL 17.11; Go go1.25.5; Compose images: quay.io/debezium/server:3.6.3.Final@sha256:e1c8e29cbb0c44f3cd9b742222e7899520ed1c210906ba071e80dbb8b987d0bd, postgres:17.11-alpine3.24@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24, cdc-crashtest-receiver.
- Stop requested at 2026-09-28T11:15:35.5241872Z UTC, completed at 2026-09-28T11:15:37.4651598Z UTC (1.941s).
- Last pre-stop sample at 2026-09-28T11:15:35.5080447Z UTC: active=true, health=`UP`, flush=`0/236ACB8`, retained=277872 bytes.
- Inactive slot: first sampled at 2026-09-28T11:15:40.5073245Z UTC (4.983s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=231192 bytes.
- Health not UP: first sampled at 2026-09-28T11:15:40.5073245Z UTC (4.983s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=231192 bytes.
- First completed-stop sample (growth baseline): first sampled at 2026-09-28T11:15:40.5073245Z UTC (4.983s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=231192 bytes.
- Retained WAL above completed-stop baseline: first sampled at 2026-09-28T11:15:45.5077169Z UTC (9.984s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=253712 bytes.
- Confirmed flush LSN: pre-stop `0/236ACB8`, first completed-stop sample `0/23AE870`, final `0/23AE870`. Restart LSN: pre-stop `0/236ACB8`, first completed-stop sample `0/237B950`, final `0/237B950`.
- Retained-WAL distance: start 26264, pre-stop 277872, end 413072, peak 413072 bytes; net growth rate after first completed-stop sample 4041.4 bytes/second.
- `pg_wal` directory size: start 33554432, end 33554432 bytes. Final health: `ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`.
- Health samples before stop: UP 10, not UP 0; after stop request: UP 0, not UP 10.

Verdict: after Debezium was stopped, the first observed slot inactivity, health failure, and post-stop retained-WAL increase occurred at the times above. Each is bounded by sampling cadence; the exact instant between samples is unknown. This demonstrates source-side retention pressure, not disk exhaustion or lost events.
The runner restarts Debezium after recording the stopped-state observations. That recovery is outside this E3 measurement window.
