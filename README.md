# CodeHound 🛡️🔍
> **Autonomous AI-Driven Repository Verification, DevSecOps, & QA Engineering Platform**

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![CI/CD](https://github.com/Santosh-Prasad-Verma/CodeHound/actions/workflows/ci.yml/badge.svg)](https://github.com/Santosh-Prasad-Verma/CodeHound/actions)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Architecture](https://img.shields.io/badge/Architecture-3--Tier_Consensus_AI-blueviolet)]()

---

## 📌 Overview

**CodeHound (ForgeGuard)** is an enterprise-grade, closed-loop software verification, DevSecOps, and QA engineering platform. Moving beyond shallow pull-request diff commenters, CodeHound operates as a **closed-loop verification platform**:
1. **Multilingual AST Parsing & Code Graphs**: Deep symbol extraction and interprocedural taint flow analysis using Tree-sitter.
2. **Deterministic Static & Secret Assurance**: Integrated Semgrep SAST, Gitleaks regex/entropy detection, TruffleHog verification, and CycloneDX/OSV vulnerability matching.
3. **3-Tier Consensus AI Reasoning Core**: Cost-optimized routing (Fast Syntactic Triage $\rightarrow$ Dual Logic & Security Analysis $\rightarrow$ Arbiter / Judge).
4. **Sandboxed Dynamic Verification**: Firecracker / E2B microVMs executing generated unit, integration, mutation, and regression tests.
5. **Self-Healing Auto-Patch Synthesis**: Synthesizes, applies, and proves bug fixes in sandboxes before opening PRs.
6. **Universal Client Surfaces**: Terminal CLI (Bubbletea TUI), Control Plane REST API, SSE real-time streaming, and MCP 2026 integration.

---

## 🏛️ Architecture & Workspace Structure

```
CodeHound/
├── cli/                 # Interactive Terminal CLI & Bubbletea TUI
├── core/                # Control Plane HTTP API, Scanner Engines, AI Orchestrator
│   ├── cmd/             # Server and database migration entrypoints
│   ├── migrations/      # PostgreSQL 18 schema migrations
│   └── pkg/             # Core packages (AST, AI, Auth, Audit, Database, Scanners)
├── shared/              # Shared domain models, UUIDv7, Enums, AppErrors
├── workers/             # Async ingestion and background AST parsing workers
├── Docs/                # Comprehensive PRDs, System Architecture, & Specs
├── docker-compose.yml   # LocalStack, PostgreSQL 18, Redis, Temporal, NATS stack
├── go.work              # Multi-module Go workspace
└── Makefile             # Developer automation commands
```

---

## ⚡ Quick Start

### 1. Prerequisites
- **Go**: `1.24+`
- **Docker & Docker Compose**: For local services (PostgreSQL 18, Redis, Temporal, NATS, LocalStack)
- **Doppler CLI**: Optional, for cloud secret injection

### 2. Local Infrastructure Setup
```bash
# 1. Clone the repository
git clone https://github.com/Santosh-Prasad-Verma/CodeHound.git
cd CodeHound

# 2. Copy environment template
cp .env.example .env

# 3. Spin up local development infrastructure
make up

# 4. Run PostgreSQL 18 schema migrations
make migrate
```

### 3. Run the Services
```bash
# Start Control Plane Server
make server

# Run interactive TUI Scanner
make cli

# Run all test suites
make test

# Compile all release binaries
make build
```

---

## 🧪 Testing & CI/CD

CodeHound includes full test suites with race detection and code coverage:
```bash
go test -v -race -cover ./shared/... ./core/... ./cli/... ./workers/...
```

GitHub Actions automatically executes the complete test matrix, schema migrations, and compilation pipeline on all pushes to `main` and `develop`.

---

## 📜 License

Licensed under the Apache License, Version 2.0.
