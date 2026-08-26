# ==============================================================================
# CodeHound Enterprise Security & Dynamic Verification Engine - Makefile
# ==============================================================================

SHELL := /bin/bash
BIN_DIR := bin

.PHONY: all help up down ps logs migrate migrate-down migrate-status migrate-reset server cli test lint build clean

help:
	@echo "CodeHound Developer Commands:"
	@echo "  make up             - Start LocalStack, Postgres, Redis, Temporal, NATS"
	@echo "  make down           - Stop all local containers"
	@echo "  make ps             - Show status of local containers"
	@echo "  make logs           - Stream container logs"
	@echo "  make migrate        - Apply PostgreSQL 18 schema migrations (UP)"
	@echo "  make migrate-down   - Rollback PostgreSQL 18 migrations (DOWN)"
	@echo "  make migrate-status - Check current database schema version"
	@echo "  make migrate-reset  - Reset database schema (DOWN -> UP)"
	@echo "  make server         - Start Control Plane API server"
	@echo "  make cli            - Launch interactive CodeHound CLI TUI"
	@echo "  make test           - Run all workspace unit and integration tests"
	@echo "  make lint           - Run Go vet and formatting checks"
	@echo "  make build          - Compile all binaries into ./bin"
	@echo "  make clean          - Remove compiled binaries and test cache"
	@echo "  make doppler-setup  - Configure Doppler CLI for project 'codehound'"
	@echo "  make doppler-server - Run Control Plane API with Doppler secrets injection"

up:
	docker compose up -d

down:
	docker compose down

ps:
	docker compose ps

logs:
	docker compose logs -f

migrate:
	go run ./core/cmd/migrate up

migrate-down:
	go run ./core/cmd/migrate down

migrate-status:
	go run ./core/cmd/migrate status

migrate-reset:
	go run ./core/cmd/migrate reset

server:
	go run ./core/cmd/server

cli:
	go run ./cli/main.go scan

test:
	go test -v -race -cover ./shared/... ./core/... ./cli/... ./workers/...

lint:
	go vet ./shared/... ./core/... ./cli/... ./workers/...

build: clean
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/codehound-server ./core/cmd/server
	go build -o $(BIN_DIR)/codehound-migrate ./core/cmd/migrate
	go build -o $(BIN_DIR)/codehound ./cli
	go build -o $(BIN_DIR)/codehound-worker ./workers/cmd/worker
	@echo "✅ All binaries built into ./$(BIN_DIR)"

clean:
	@rm -rf $(BIN_DIR)
	@go clean -testcache
	@echo "🧹 Cleaned build artifacts."

doppler-setup:
	doppler setup --project codehound --config dev

doppler-server:
	doppler run -- go run ./core/cmd/server

