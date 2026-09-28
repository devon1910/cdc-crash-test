# E2: noise-only workload

- Noise: 5999 committed rows at requested 10/s for 10m0s (actual load 10m0.018s). Orders written by runner: 0.
- Observer: 122 samples every 5s plus 5s settle; largest gap 5.207s (limit 12s).
- Slot: `cdc_crashtest_slot`; active throughout: true; Debezium healthy throughout: true.
- Confirmed flush LSN: `0/1ACD460` to `0/1DC9F80`, advanced: true.
- Restart LSN: `0/1AC83D8` to `0/1D8A808`.
- Retained-WAL distance: 1635704 to 375960 bytes; peak 1924632 bytes.
- pg_wal on-disk size: 33554432 to 33554432 bytes.
- Heartbeat row updates during run: 59.
- Debezium setting: 10-second heartbeat plus published action query.

Verdict: observation complete. Compare this run with the other scenario; LSN advance alone does not prove end-to-end event delivery. WAL distance is from the slot's restart position; pg_wal size is allocated files on disk.
