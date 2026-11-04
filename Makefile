SHELL := /bin/sh
COMPOSE ?= docker compose
DEV_COMPOSE := $(COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml

.PHONY: help up down logs ps restart build test test-api test-frontend lint fmt clean \
	db-shell db-backup db-restore dev dev-deps fetch-model fetch-onnxruntime smoke config

help:
	@echo "Sutra developer commands"
	@echo ""
	@echo "  make up               Start the full Docker stack (build if needed)"
	@echo "  make down             Stop containers without deleting volumes"
	@echo "  make logs             Follow all service logs"
	@echo "  make ps               Show service status"
	@echo "  make restart          Restart application containers"
	@echo "  make build            Build API and frontend images"
	@echo "  make test             Run backend tests and frontend typecheck"
	@echo "  make lint             Run go vet, TypeScript typecheck and compose validation"
	@echo "  make fmt              Format Go sources"
	@echo "  make clean            Remove generated build artifacts (keeps volumes/data)"
	@echo "  make dev              Run PostgreSQL in Docker, API and Vite on the host"
	@echo "  make db-shell         Open psql in the database container"
	@echo "  make db-backup        Write a compressed dump to backups/"
	@echo "  make db-restore FILE=backups/....dump"
	@echo "  make smoke            Run the end-to-end HTTP smoke test"
	@echo "  make config           Validate docker-compose.yml"

up:
	$(COMPOSE) up --build -d
	@echo ""
	@echo "Sutra is starting. Open http://localhost:$${FRONTEND_PORT:-3000}"
	@echo "Follow startup with: $(COMPOSE) logs -f"

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

ps:
	$(COMPOSE) ps

restart:
	$(COMPOSE) restart api frontend

build:
	$(COMPOSE) build

test: test-api test-frontend

test-api:
	cd api && go test ./...

test-frontend:
	cd frontend && npm ci && npm run typecheck

lint:
	cd api && go vet ./...
	cd frontend && npm run typecheck
	$(COMPOSE) config --quiet

fmt:
	cd api && gofmt -w .

clean:
	rm -rf frontend/dist
	rm -f frontend/*.tsbuildinfo frontend/tsconfig.*.tsbuildinfo
	rm -f api/sutra api/backfill
	@echo "Removed generated build artifacts. Named Docker volumes and data are untouched."

db-shell:
	$(COMPOSE) exec db sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'

db-backup:
	mkdir -p backups
	$(COMPOSE) exec -T db sh -c 'pg_dump -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -Fc' > "backups/sutra-$$(date -u +%Y%m%d-%H%M%S).dump"
	@ls -1t backups/*.dump | head -n1

db-restore:
	@if [ -z "$(FILE)" ]; then echo "Usage: make db-restore FILE=backups/sutra-YYYYMMDD-HHMMSS.dump"; exit 1; fi
	@test -f "$(FILE)" || (echo "Backup not found: $(FILE)"; exit 1)
	$(COMPOSE) exec -T db sh -c 'pg_restore -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" --clean --if-exists --no-owner' < "$(FILE)"

dev-deps:
	@test -f api/embedding/models/bge-small-en-v1.5/model.onnx || ./scripts/fetch-model.sh
	@test -f api/third_party/onnxruntime/onnxruntime-linux-x64-1.31.1/lib/libonnxruntime.so || ./scripts/fetch-onnxruntime.sh
	@test -d frontend/node_modules || (cd frontend && npm ci)
	@cd api && go mod download

dev: dev-deps
	$(DEV_COMPOSE) up -d db
	@echo "PostgreSQL is listening on 127.0.0.1:$${POSTGRES_PORT:-5432}."
	@echo "API: http://localhost:8080  Frontend: http://localhost:5173"
	@set -e; \
	( cd api && DATABASE_URL="$${DATABASE_URL:-postgresql://searcher:searcher@127.0.0.1:5432/searcher}" go run ./cmd/server ) & api_pid=$$!; \
	( cd frontend && npm run dev ) & web_pid=$$!; \
	trap 'kill $$api_pid $$web_pid 2>/dev/null || true' INT TERM EXIT; \
	wait

fetch-model:
	./scripts/fetch-model.sh

fetch-onnxruntime:
	./scripts/fetch-onnxruntime.sh

smoke:
	./scripts/smoke-test.sh

config:
	$(COMPOSE) config --quiet
	$(COMPOSE) --profile artifacts config --quiet
	@echo "docker-compose.yml is valid"
