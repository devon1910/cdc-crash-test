.PHONY: up down reset logs ps e0 e1 e2

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
