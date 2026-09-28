.PHONY: up down reset logs ps e0 e1 e2 e2-timer heartbeat-comparison m4-comparison e3 disk-fill

up:
	docker compose up --build -d

down:
	docker compose down

reset:
	docker compose down --volumes --remove-orphans

logs:
	docker compose logs -f debezium receiver postgres

ps:
	docker compose ps

e0:
	go run ./cmd/e0 $(E0_ARGS)

e1:
	go run ./cmd/heartbeat -scenario=e1 $(E1_ARGS)

e2:
	go run ./cmd/heartbeat -scenario=e2 $(E2_ARGS)

e2-timer:
	go run ./cmd/heartbeat -scenario=e2-timer $(E2_TIMER_ARGS)

# Deletes Compose volumes after an interactive confirmation.
heartbeat-comparison:
	go run ./cmd/heartbeatcomparison

m4-comparison: heartbeat-comparison

e3:
	go run ./cmd/e3 $(E3_ARGS)

disk-fill:
	go run ./cmd/diskfill -scenario=no-heartbeat $(DISK_FILL_ARGS)
	go run ./cmd/diskfill -scenario=action-query $(DISK_FILL_ARGS)
