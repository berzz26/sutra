.PHONY: build-backend run-backend compose-up compose-down test tidy

build-api:
	cd api && go build -o sutra ./cmd/server

# Loads variables from .env into the environment (requires .env; see .env.example).
run-api:
	cd api && set -a && . ../.env && set +a && go run ./cmd/server

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

test:
	cd api && go test ./...
	cd frontend && npm run typecheck

tidy:
	cd api && go mod tidy
