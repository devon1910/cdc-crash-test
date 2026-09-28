# E3: connector stopped while writes continue

Question: how soon after Debezium stops do slot inactivity, health failure, and retained-WAL growth appear?

- Workload: `orders` and `noise`, each requested at 10 inserts/second for 1m30s, 4096-byte noise payload; committed 899 orders and 899 noise rows (actual load 1m30.001s).
- Observer: 19 samples at 5s interval plus 5s settle; largest sample gap 10.001s (coverage limit 12s).
- Slot: `cdc_crashtest_slot`. Debezium config: heartbeat disabled.
- Stop requested at 2026-09-28T10:46:30.542721Z UTC, completed at 2026-09-28T10:46:32.3823065Z UTC (1.84s).
- Last pre-stop sample at 2026-09-28T10:46:30.5339246Z UTC: active=true, health=`UP`, flush=`0/1F938A0`, retained=241416 bytes.
- Inactive slot: first sampled at 2026-09-28T10:46:35.5345431Z UTC (4.992s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=226816 bytes.
- Health not UP: first sampled at 2026-09-28T10:46:35.5345431Z UTC (4.992s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=226816 bytes.
- First completed-stop sample (growth baseline): first sampled at 2026-09-28T10:46:35.5345431Z UTC (4.992s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=226816 bytes.
- Retained WAL above completed-stop baseline: first sampled at 2026-09-28T10:46:40.5337074Z UTC (9.991s after stop request); active=false, health=`ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`, retained=249352 bytes.
- Confirmed flush LSN: pre-stop `0/1F938A0`, final `0/1FC9F20` (advanced after pre-stop sample: true). Restart LSN: pre-stop `0/1F8F490`, final `0/1F98558`.
- Retained-WAL distance: start 520, pre-stop 241416, end 436392, peak 436392 bytes; net growth rate from pre-stop sample to end 3899.4 bytes/second.
- `pg_wal` directory size: start 33554432, end 33554432 bytes. Final health: `ERROR: Get "http://localhost:8080/q/health": dial tcp [::1]:8080: connectex: No connection could be made because the target machine actively refused it.`.

Verdict: after Debezium was stopped, the first observed slot inactivity, health failure, and post-stop retained-WAL increase occurred at the times above. Each is bounded by sampling cadence; the exact instant between samples is unknown. This demonstrates source-side retention pressure, not disk exhaustion or lost events.
The runner restarts Debezium after recording the stopped-state observations. That recovery is outside this E3 measurement window.
