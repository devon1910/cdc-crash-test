# M6 acceptance run

On 2026-09-28, the documented full sequence was run from a fresh local Git clone using the PowerShell/Go equivalents (GNU Make is not installed on this machine). The temporary clone used its own Compose project name so PostgreSQL and Debezium received new named volumes without touching the original lab's volumes. No `down --volumes` or reset was used.

Sequence: `docker compose up --build -d`, `go run ./cmd/e0`, `go run ./cmd/heartbeat -scenario=e1`, `go run ./cmd/heartbeat -scenario=e2`, and `go run ./cmd/e3`. Each experiment ran for its default 10 minutes and wrote both expected files. E0 passed its baseline rule. E1 and E2 completed with 121 and 122 samples respectively and healthy, active slots. E3's first attempt had a 25-minute gap during a delayed tool poll; its summary correctly failed coverage. That exposed an E3 timing boundary: a sample during graceful shutdown could be reported as a stopped-state transition. The runner now classifies inactivity and health only from samples taken after Docker confirms the stop completed. A new full E3 run passed with 122 samples and a 5.397-second maximum gap.

Results copied from the fresh clone:

- [E0 summary](../results/acceptance/20260928-clean-checkout/e0/summary.md) and [observations](../results/acceptance/20260928-clean-checkout/e0/observations.csv)
- [E1 summary](../results/acceptance/20260928-clean-checkout/e1/summary.md) and [observations](../results/acceptance/20260928-clean-checkout/e1/observations.csv)
- [E2 summary](../results/acceptance/20260928-clean-checkout/e2/summary.md) and [observations](../results/acceptance/20260928-clean-checkout/e2/observations.csv)
- [E3 summary](../results/acceptance/20260928-clean-checkout/e3/summary.md) and [observations](../results/acceptance/20260928-clean-checkout/e3/observations.csv)

The initial E3 attempt's incomplete files are not included in this accepted set; they remain in the temporary checkout. The Compose project name was changed only in that temporary checkout to isolate volumes and container names. The service configuration, ports, images, initialization SQL, and Go commands otherwise followed the README. The host used direct commands because GNU Make was unavailable.
