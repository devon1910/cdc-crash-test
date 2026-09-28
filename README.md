# CDC crash test (v0.1)

A local, reproducible lab for seeing what PostgreSQL retains when a Debezium CDC source goes quiet or stops. It was motivated by a real failure in which an inactive Debezium slot retained WAL until the disk filled. This lab **does not** intentionally fill a disk: it runs short, scaled-down experiments and records the slot's WAL position, health, and file usage.

The question is: **when does the slot stop advancing, does an active heartbeat help, and which signals reveal the problem?**

```text
PostgreSQL 17 -- published orders + heartbeat changes --> Debezium Server 3.6 --> HTTP receiver
      |                            |
      |-- noise writes make WAL, but noise is not published
      `-- replication slot + WAL metrics --> Go observer --> CSV + Markdown summaries
```

`public.orders` is the application table captured by Debezium. `public.noise` generates WAL but is deliberately excluded from the publication. `public.cdc_heartbeat` is a one-row synthetic table included in the publication so E2 can make a real published change every ten seconds. The receiver logs HTTP events; it is not a durable sink or a completeness checker.

## Requirements and safety

- Docker Engine/Desktop with the `docker compose` plugin running.
- Go 1.25 or newer on the host; GNU Make is optional on Windows.
- Free host ports 55432 (PostgreSQL), 8080 (Debezium health), and 8081 (receiver). All three are published to loopback only.
- Internet access the first time Docker pulls images or Go downloads modules. Each default experiment takes about 10 minutes, so the four-run sequence takes about 40 minutes plus startup.

This is **local-only** configuration. PostgreSQL uses `trust` authentication with no password, including on the Compose network; its host port is bound to `127.0.0.1`. Do not put this stack on a shared network or reuse its database configuration in production. `make reset` removes the named PostgreSQL and Debezium volumes and permanently discards their lab data and offsets. Ordinary `make down` keeps those volumes.

The PostgreSQL and Debezium images in [compose.yaml](compose.yaml) and the Go build image in [the receiver Dockerfile](cmd/receiver/Dockerfile) are pinned by tag and digest. Current tags are PostgreSQL `17.11-alpine3.24`, Debezium Server `3.6.3.Final`, and Go builder `1.25.5-alpine3.22`; the Go module requests Go `1.25.0` and pgx/v5 `v5.11.0`. The receiver runtime image is `scratch`.

## Run from a fresh clone

From the repository root, with Docker running:

```sh
make up
make e0
make e1
make e2
make e3
```

Run the experiments **sequentially**, not in parallel. Each command prints its timestamped result directory. `make up` builds the receiver, creates the PostgreSQL database and publication on a fresh volume, and starts Debezium. E1 and E2 switch the Debezium config and preserve the database, slot, and offsets. E3 stops Debezium halfway through, measures the stopped period, and restarts it afterward. Do not interrupt E3 between its stop and restart; if you do, recover with `docker compose up -d debezium`.

On Windows PowerShell without GNU Make, the equivalent commands are:

```powershell
docker compose up --build -d
go run ./cmd/e0
go run ./cmd/m4 -scenario=e1
go run ./cmd/m4 -scenario=e2
go run ./cmd/e3
```

For a quick setup check, shorten each run using `-duration=90s`; keep the defaults for the actual 10-minute comparisons. E0/E1/E2 also accept `-interval=5s`, `-settle=5s`, and `-rate=10`; E3 uses the same flags and stops Debezium halfway through the chosen duration. A short run checks wiring, **not** the full experiment verdict. If port 55432 conflicts, change the Compose port mapping and pass the matching `-dsn` to each Go command; the default host DSN is `postgres://postgres@127.0.0.1:55432/cdc_lab?sslmode=disable`.

To check the running stack:

```powershell
docker compose ps
(Invoke-RestMethod http://localhost:8080/q/health).status
```

Expect PostgreSQL and receiver healthy, Debezium running, and health `UP` except during E3's deliberately stopped interval. `docker compose logs receiver` shows delivered events. For deeper checks, see the [observer](docs/m2-observer.md), [E0](docs/m3-e0.md), [E1/E2](docs/m4-e1-e2.md), and [E3](docs/m5-e3.md) notes, plus the [ownership Q&A](docs/defend-it-questions-and-answers.md).

## Experiments and what to expect

| Run | Writes | Debezium | Question / expected signal |
| --- | --- | --- | --- |
| E0 baseline | `orders` | Running, heartbeat off | Does the slot acknowledge changes and keep retained-WAL distance small? |
| E1 quiet source | `noise` only | Running, heartbeat off | Can the slot appear healthy while its LSNs stall and WAL distance grows? |
| E2 active heartbeat | `noise` only | Running; heartbeat action updates `cdc_heartbeat` | Do slot LSNs advance and WAL distance stay lower than in E1? |
| E3 connector stopped | `orders` and `noise` | Stopped halfway through | When do the slot become inactive, health fail, and WAL distance rise? |

Each run writes `results/e0`, `results/e1`, `results/e2`, or `results/e3` followed by a UTC timestamp directory containing `observations.csv` and `summary.md`. Read each summary first, then inspect its CSV if a result is surprising. E0's pass rule is a **lab heuristic**, not a production safety threshold. E1 and E2 should be compared by changes during each run, not absolute starting LSNs; the connector is restarted between them. E3 reports sampled time bounds, not the exact instant each signal changed. The measured full-run [E1/E2 comparison](results/m4-comparison.md) and [E3 summary](results/e3/20260928T104944.136Z/summary.md) are examples, not guaranteed numbers for another machine.

## Reading `observations.csv`

An LSN (Log Sequence Number) is a position in PostgreSQL's write-ahead log (WAL). A replication slot retains WAL that its consumer may still need. The two most useful columns are `confirmed_flush_lsn` (where Debezium has acknowledged processing) and `restart_lsn` (the oldest WAL the slot might still require). They can move differently.

| Column | Meaning |
| --- | --- |
| `timestamp` | UTC time the sample began. |
| `slot_name` | Name of the PostgreSQL replication slot. |
| `active` | Whether a replication client is attached at this sample. `true` does not prove it is keeping up. |
| `restart_lsn` | Oldest WAL position the slot might still need; important for retention. |
| `confirmed_flush_lsn` | WAL position the logical consumer confirmed. Advancement is source-side progress, not proof of sink delivery. |
| `wal_status` | PostgreSQL's status of WAL required by the slot. |
| `retained_wal_bytes` | Byte distance from `restart_lsn` to current WAL LSN; a slot-pressure indicator, **not** disk usage. Blank if no restart LSN exists. |
| `pg_wal_bytes` | Actual bytes in current files under PostgreSQL's `pg_wal` directory. Files are allocated/recycled by segment, so this need not track LSN distance closely. |
| `debezium_health_status` | Debezium `/q/health` aggregate result, or an HTTP/network error. `UP` alone does not mean the slot is advancing. |
| `checkpoint_count` | Cumulative timed plus requested checkpoints; compare successive rows to see whether a checkpoint occurred. |

More beginner-friendly definitions are in [CDC terms](docs/cdc-terms.md). The column definitions and PostgreSQL references are in [observer notes](docs/m2-observer.md).

## Limits and cleanup

This uses one laptop, self-hosted PostgreSQL 17, Debezium **Server** (not Kafka Connect), a non-durable HTTP log receiver, and small WAL/data rates. It does not test a managed database, other CDC tools, standbys, failover, replication-slot invalidation, `max_slot_wal_keep_size`, a 30 GB incident, exact-once delivery, or a data-loss ledger. The lab cannot tell you that every event arrived or that a production disk will be safe. Treat its results as measured behavior under these specific settings.

Use `make down` (or `docker compose down`) to stop the lab while preserving volumes. Use `make up` to resume. **Only when you intentionally want to erase the lab state**, use `make reset` (`docker compose down --volumes --remove-orphans`). Existing result files in `results/` are ordinary files and are not removed by either command.

## Verification status

On 2026-09-28, the documented default-duration E0 → E1 → E2 → E3 sequence completed from a fresh local clone using the PowerShell/Go commands above. E0 passed its baseline rule; E1, E2, and E3 generated complete observations and summaries; E3 restarted Debezium, whose health returned `UP`. The run artifacts and any test limitations are recorded in [M6 acceptance](docs/m6-acceptance.md). `go test ./...`, `go vet ./...`, `docker compose config --quiet`, and the short sequential smoke run also passed. GNU Make was unavailable, so its documented command equivalents were used.
