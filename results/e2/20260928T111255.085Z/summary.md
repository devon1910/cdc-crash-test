# E2: noise-only workload

- Noise: 899 committed rows at requested 10/s for 1m30s (actual load 1m30.009s). Orders written by runner: 0.
- Observer: 20 samples every 5s plus 5s settle; largest gap 5.023s (limit 12s).
- Slot: `cdc_crashtest_slot`; active throughout: true; Debezium healthy throughout: true.
- Confirmed flush LSN: `0/22F6188` to `0/235DC20`, advanced: true.
- Restart LSN: `0/22F4C08` to `0/23404C8`.
- Retained-WAL distance: 240280 to 173920 bytes; peak 415384 bytes.
- pg_wal on-disk size: 33554432 to 33554432 bytes.
- Heartbeat row updates during run: 9.
- Debezium setting: 10-second heartbeat plus published action query.

Verdict: observation complete. Compare this run with the other scenario; LSN advance alone does not prove end-to-end event delivery. WAL distance is from the slot's restart position; pg_wal size is allocated files on disk.
