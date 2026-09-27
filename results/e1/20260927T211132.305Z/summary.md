# E1: noise-only workload

- Noise: 5999 committed rows at requested 10/s for 10m0s (actual load 10m0.007s). Orders written by runner: 0.
- Observer: 122 samples every 5s plus 5s settle; largest gap 5.028s (limit 12s).
- Slot: `cdc_crashtest_slot`; active throughout: true; Debezium healthy throughout: true.
- Confirmed flush LSN: `0/1C7DCB8` to `0/1C7DCB8`, advanced: false.
- Restart LSN: `0/1C7CF90` to `0/1C7CF90`.
- Retained-WAL distance: 3960 to 1609248 bytes; peak 1609248 bytes.
- pg_wal on-disk size: 33554432 to 33554432 bytes.
- Heartbeat row updates during run: 0.
- Debezium setting: heartbeats disabled.

Verdict: observation complete. Compare this run with the other scenario; LSN advance alone does not prove end-to-end event delivery. WAL distance is from the slot's restart position; pg_wal size is allocated files on disk.
