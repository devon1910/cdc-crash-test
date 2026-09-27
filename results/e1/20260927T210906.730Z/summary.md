# E1: noise-only workload

- Noise: 249 committed rows at requested 10/s for 25s (actual load 25.016s). Orders written by runner: 0.
- Observer: 7 samples every 5s plus 5s settle; largest gap 5.271s (limit 12s).
- Slot: `cdc_crashtest_slot`; active throughout: true; Debezium healthy throughout: true.
- Confirmed flush LSN: `0/1C2D200` to `0/1C2D200`, advanced: false.
- Restart LSN: `0/1C2CF48` to `0/1C2CF48`.
- Retained-WAL distance: 172552 to 234232 bytes; peak 234232 bytes.
- pg_wal on-disk size: 33554432 to 33554432 bytes.
- Heartbeat row updates during run: 0.
- Debezium setting: heartbeats disabled.

Verdict: observation complete. Compare this run with the other scenario; LSN advance alone does not prove end-to-end event delivery. WAL distance is from the slot's restart position; pg_wal size is allocated files on disk.
