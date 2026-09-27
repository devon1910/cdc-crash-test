.PHONY: up down reset logs ps e0

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
