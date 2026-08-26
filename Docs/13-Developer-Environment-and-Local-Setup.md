# Developer Environment & Local Setup Guide — CodeHound (ForgeGuard)

**Document:** 13-Developer-Environment-and-Local-Setup.md  
**Status:** Approved Specification  
**Target:** Local Development Workflow, Docker Compose Stack, Mock Sandboxes & Contributing  
**Date:** 2026-08-25  

---

## 1. Prerequisites

To build and run the CodeHound platform locally:
- **Go**: `1.23+` (for Control Plane, Ingestion, and CLI)
- **Node.js / pnpm**: `Node 22+`, `pnpm 9+` (for SvelteKit Console and Web interfaces)
- **Docker & Docker Compose**: For local infrastructure stack (PostgreSQL, Temporal, NATS, Qdrant)
- **Rust / Cargo**: Optional, required only for modifying the low-level microVM jailer daemon.

---

## 2. One-Command Local Infrastructure (`docker-compose.yml`)

A unified `docker-compose.yml` spins up all core services locally:

```yaml
version: '3.9'

services:
  postgres:
    image: postgres:18-alpine
    environment:
      POSTGRES_DB: codehound_dev
      POSTGRES_USER: codehound
      POSTGRES_PASSWORD: devpassword
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  qdrant:
    image: qdrant/qdrant:v1.11.0
    ports:
      - "6333:6333"
      - "6334:6334"

  nats:
    image: nats:2.10-alpine
    command: ["-js", "-m", "8222"]
    ports:
      - "4222:4222"
      - "8222:8222"

  temporal:
    image: temporalio/auto-setup:1.24.2
    environment:
      - DB=postgresql
      - DB_PORT=5432
      - POSTGRES_USER=codehound
      - POSTGRES_PWD=devpassword
      - POSTGRES_SEEDS=postgres
    ports:
      - "7233:7233"
      - "8233:8233"

  temporal-ui:
    image: temporalio/ui:2.28.0
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
    ports:
      - "8080:8080"

volumes:
  pgdata:
```

---

## 3. Quickstart Commands

```bash
# 1. Start local dependencies
docker compose up -d

# 2. Run database migrations
cd core && go run ./cmd/migrate up

# 3. Start the Control API
cd core && go run ./cmd/server

# 4. Start the Temporal Workers in Simulation Mode (uses Docker instead of Firecracker on macOS/Windows)
cd core && SIMULATION_MODE=true go run ./cmd/worker

# 5. Launch the Web Console
cd web && pnpm install && pnpm dev

# 6. Run a test audit using the local CLI
cd cli && go run ./main.go scan ../sample-repo --local
```
