# ==============================================================================
# ScanDrix Enterprise Engineering Makefile
# ==============================================================================

.PHONY: all build test test-race bench clean docker-up docker-down fmt lint run-api run-worker run-webhooks

BIN_DIR := ./bin
APPS := api server webhooks worker cli mcp-manager ast-cli analytics-cli try migrate

all: build test

build:
	@mkdir -p $(BIN_DIR)
	@echo "Building all 10 ScanDrix binaries..."
	go build -o $(BIN_DIR)/scandrix-api ./cmd/api
	go build -o $(BIN_DIR)/scandrix-server ./cmd/server
	go build -o $(BIN_DIR)/scandrix-webhooks ./cmd/webhooks
	go build -o $(BIN_DIR)/scandrix-worker ./cmd/worker
	go build -o $(BIN_DIR)/scandrix-cli ./cmd/cli
	go build -o $(BIN_DIR)/scandrix-mcp-manager ./cmd/mcp-manager
	go build -o $(BIN_DIR)/scandrix-ast-cli ./cmd/ast-cli
	go build -o $(BIN_DIR)/scandrix-analytics-cli ./cmd/analytics-cli
	go build -o $(BIN_DIR)/scandrix-try ./cmd/try
	go build -o $(BIN_DIR)/scandrix-migrate ./cmd/migrate
	@echo "All 10 binaries compiled successfully into $(BIN_DIR)/"

test:
	@echo "Running unit and integration tests..."
	go test -v ./...

test-race:
	@echo "Running test suite with race detector (-race)..."
	go test -v -race ./...

bench:
	@echo "Running micro-benchmarks with memory profiling..."
	go test -bench=. -benchmem ./test/benchmark/...

docker-up:
	@echo "Starting full ScanDrix cluster..."
	docker compose up -d

docker-down:
	@echo "Stopping ScanDrix cluster..."
	docker compose down

fmt:
	@echo "Formatting Go source files..."
	go fmt ./...

lint:
	@echo "Checking Go code vet..."
	go vet ./...

clean:
	@echo "Cleaning binaries..."
	rm -rf $(BIN_DIR)
