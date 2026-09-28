.PHONY: up down reset logs ps e0 e1 e2 e2-timer m4-comparison e3

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
	go run ./cmd/m4 -scenario=e1 $(E1_ARGS)

e2:
	go run ./cmd/m4 -scenario=e2 $(E2_ARGS)

e2-timer:
	go run ./cmd/m4 -scenario=e2-timer $(E2_TIMER_ARGS)

# Destructive to Compose volumes: intended for isolated experiment comparisons.
m4-comparison:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/run-m4-comparison.ps1

e3:
	go run ./cmd/e3 $(E3_ARGS)
