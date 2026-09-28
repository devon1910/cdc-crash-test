# E1: noise-only workload

- Noise: 899 committed rows at requested 10/s for 1m30s (actual load 1m30.008s). Orders written by runner: 0.
- Observer: 20 samples every 5s plus 5s settle; largest gap 5.026s (limit 12s).
- Slot: `cdc_crashtest_slot`; active throughout: true; Debezium healthy throughout: true.
- Confirmed flush LSN: `0/22F6188` to `0/22F6188`, advanced: false.
- Restart LSN: `0/22F4C08` to `0/22F4C08`.
- Retained-WAL distance: 12960 to 239832 bytes; peak 239832 bytes.
- pg_wal on-disk size: 33554432 to 33554432 bytes.
- Heartbeat row updates during run: 0.
- Debezium setting: heartbeats disabled.

Verdict: observation complete. Compare this run with the other scenario; LSN advance alone does not prove end-to-end event delivery. WAL distance is from the slot's restart position; pg_wal size is allocated files on disk.
