# 🛡️ ScanDrix

**ScanDrix** is an enterprise-grade, high-performance automated code review and security analysis platform built in clean-room Go 1.24. Designed for scale, zero-trust security, and adversarial precision, ScanDrix orchestrates multi-agent code analysis, deep AST dependency graphs, and cryptographic supply-chain provenance.

---

## ⚡ Key Capabilities

- **🚀 9 Clean-Room Go Binaries**:
  - `scandrix-api`: REST API gateway powered by Chi router, RBAC guards, and SSE event streaming.
  - `scandrix-worker`: High-throughput background processor executing 9-stage code review pipelines.
  - `scandrix-webhooks`: High-performance webhook ingestion daemon capable of handling **13,833 req/sec**.
  - `scandrix-cli`: Terminal-first code review tool outputting OASIS SARIF v2.1.0 and colored diff annotations.
  - `scandrix-mcp-manager`: Model Context Protocol (MCP) JSON-RPC 2.0 server.
  - `scandrix-ast-cli`: Halstead complexity, cyclomatic $v(G)$, and cognitive nesting analyzer.
  - `scandrix-analytics-cli`: Continuous DORA engineering metrics and delivery velocity calculator.
  - `scandrix-try`: Interactive sandbox playground for zero-install dry-run previews.
  - `scandrix-server`: Unified standalone all-in-one daemon for air-gapped deployments.

- **🧠 Multi-Agent Adversarial Deliberation**:
  - 4-persona review panel (`Security Agent`, `Performance Agent`, `Bug Agent`, `Style Agent`).
  - Adversarial second-pass verifier agent cutting false positives by over 70%.
  - Mathematical file blast-radius scoring ($\text{Score} = \text{diffMult} \times \text{statusMult} \times \text{structuralWeight}$).

- **🛡️ Enterprise Zero-Trust & Security**:
  - NIST SP 800-57 2-Tier KMS Envelope Encryption.
  - In-toto SLSA v1.0 Cryptographic Attestations with DSSE Pre-Authentication Encoding.
  - RFC 5424 Syslog & HP CEF v0 SIEM audit logging with tamper-evident SHA-256 hash chains.
  - Ed25519 digital licensing and SCIM 2.0 enterprise directory sync.

---

## 🏗️ Architecture Overview

```
                                  +---------------------------------------+
                                  |     SCM Webhook / Developer PR        |
                                  | (GitHub, GitLab, Bitbucket, Azure)    |
                                  +-------------------+-------------------+
                                                      |
                                                      v
                                        +-------------+-------------+
                                        |    scandrix-webhooks      |
                                        | (HMAC validation, Outbox) |
                                        +-------------+-------------+
                                                      |
                                                      v
                                        +-------------+-------------+
                                        |    RabbitMQ Quorum Queue  |
                                        +-------------+-------------+
                                                      |
                                                      v
                                        +-------------+-------------+
                                        |      scandrix-worker      |
                                        |  (9-Stage Review Pipeline)|
                                        +-------------+-------------+
                                                      |
                   +----------------------------------+----------------------------------+
                   |                                  |                                  |
                   v                                  v                                  v
+------------------+------------------+ +-------------+-------------+ +------------------+------------------+
|      Multi-Model LLM Gateway        | |      AST Graph Analyzer   | |       Supabase PostgreSQL 17       |
| (OpenAI, Claude, Gemini, BYOK)      | |  (Call Graphs & Halstead) | |   (pgvector HNSW Security Index)   |
+-------------------------------------+ +---------------------------+ +-------------------------------------+
```

---

## 🚀 Getting Started

### Prerequisites
- **Go 1.24+**
- **Docker & Docker Compose**
- **PostgreSQL 17** (with `pgvector` extension)

### Local Development
```bash
# 1. Clone repository
git clone https://github.com/Santosh-Prasad-Verma/CodeHound.git
cd CodeHound

# 2. Configure environment
cp .env.example .env

# 3. Compile all 9 binaries
make build

# 4. Run full test suite with race detector (0 data races)
go test -race ./...

# 5. Start development cluster
docker compose up -d
```

---

## 🧪 Verification & Quality Standards

Every pull request and build is rigorously tested:
```bash
# Run unit and race detection tests
go test -v -race ./...

# Run end-to-end integration tests
go test -v -race ./test/integration/...

# Run microbenchmarks
go test -bench=. ./test/benchmark/...
```

---

## 📄 License
Enterprise Proprietary & Open Source Dual License. All rights reserved.
