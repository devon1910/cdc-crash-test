# E2: noise-only workload

- Noise: 326 committed rows at requested 10/s for 35s (actual load 35.018s). Orders written by runner: 0.
- Observer: 9 samples every 5s plus 5s settle; largest gap 5.293s (limit 12s).
- Slot: `cdc_crashtest_slot`; active throughout: true; Debezium healthy throughout: true.
- Confirmed flush LSN: `0/1C2D200` to `0/1C2D970`, advanced: true.
- Restart LSN: `0/1C2CF48` to `0/1C2CF48`.
- Retained-WAL distance: 234680 to 327408 bytes; peak 327408 bytes.
- pg_wal on-disk size: 33554432 to 33554432 bytes.
- Heartbeat row updates during run: 4.
- Debezium setting: 10-second heartbeat plus published action query.

Verdict: observation complete. Compare this run with the other scenario; LSN advance alone does not prove end-to-end event delivery. WAL distance is from the slot's restart position; pg_wal size is allocated files on disk.
