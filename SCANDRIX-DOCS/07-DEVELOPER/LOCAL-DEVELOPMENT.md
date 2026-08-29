# Scandrix — Local Developer Setup Guide

**Classification:** NORMATIVE DEVELOPER GUIDE  
**Status:** APPROVED  
**Target Version:** v1.0 Enterprise  
**Toolchain:** Go 1.24+, Node.js 22 LTS, pnpm, Docker Engine, Doppler CLI

---

## 1. Prerequisites & Toolchain Verification

Before configuring the local environment, ensure the following core runtimes are installed:

```bash
# Verify Go 1.24+
go version
# Expect: go version go1.24.0 linux/amd64

# Verify Node.js 22 LTS & pnpm
node -v && pnpm -v
# Expect: v22.x.x and 9.x.x

# Verify Docker & gVisor runsc runtime
docker info | grep -i runsc
# Expect: Runtimes: runsc runc
```

---

## 2. Infrastructure Setup (Docker Compose)

Scandrix utilizes local containerized instances of PostgreSQL (with `pgvector`), Redis / Valkey, RabbitMQ (with management plugin), and Appwrite for local development:

```bash
# 1. Clone repository
git clone https://github.com/scandrix/scandrix.git
cd scandrix

# 2. Configure local environment variables
cp .env.example .env

# 3. Start backing infrastructure
docker compose -f docker-compose.dev.yml up -d

# 4. Verify healthy service status
docker compose -f docker-compose.dev.yml ps
```

---

## 3. Database Migration & Seed Data

Apply the initial schema migrations and seed standard test organizations and rules:

```bash
# Run schema migrations via Goose / golang-migrate
go run cmd/scandrix-migrate/main.go up

# Seed test tenant and default security policies
go run cmd/scandrix-seed/main.go
```

---

## 4. Running Backend Services & Frontend Dashboard

Open three terminal tabs to run the services in development mode:

### Tab 1: API Server
```bash
go run cmd/scandrix-api/main.go
# Server listening on http://localhost:8080 (REST) and :9090 (gRPC)
```

### Tab 2: Analysis Worker Daemon
```bash
go run cmd/scandrix-worker/main.go
# Worker pool listening on RabbitMQ queue 'q.jobs.scan.dag'
```

### Tab 3: Next.js 15 Web Dashboard
```bash
cd web
pnpm install
pnpm dev
# Dashboard running at http://localhost:3000
```

---

## 5. Simulating Local Scans with the Scandrix CLI

You can verify the entire end-to-end review pipeline on local staged changes without pushing to GitHub:

```bash
# Build the local CLI
go build -o bin/scandrix cmd/scandrix-cli/main.go

# Run staged diff analysis
./bin/scandrix review --staged --api-url http://localhost:8080 --key scandrix_dev_test_key
```
