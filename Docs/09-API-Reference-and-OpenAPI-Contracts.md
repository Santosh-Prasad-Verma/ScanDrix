# API Reference & gRPC/Protobuf Specifications — CodeHound (ForgeGuard)

**Document:** 09-API-Reference-and-OpenAPI-Contracts.md  
**Status:** Approved Specification  
**Target:** Control API, Worker gRPC Protocol, SSE Streaming & Webhook Contracts  
**Date:** 2026-08-25  

---

## 1. API Architecture Overview

CodeHound provides three primary communication layers:
1. **REST / HTTP/2 Gateway (`/api/v1`)**: Client management, repository configuration, audit triggers, and report retrieval.
2. **Real-Time Event Stream (SSE / WebSocket at `/api/v1/stream`)**: Sub-second telemetry and progress updates for CLI, Web, and IDEs.
3. **Internal gRPC Worker Protocol**: High-throughput communication between Temporal orchestrators, Firecracker sandbox managers, and analyzer workers.

---

## 2. Core REST Endpoints (`/api/v1`)

### 2.1 Audit & Scan Management

#### `POST /api/v1/audits`
Initiates a new audit run against a repository or commit snapshot.

**Request Body:**
```json
{
  "project_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "commit_sha": "7f83b2a1c9e8d4f6a5b0c1e2d3f4a5b6c7d8e9f0",
  "scan_type": "FULL", // "FULL" | "DIFF_AWARE" | "SECURITY_ONLY" | "PERFORMANCE"
  "base_ref": "main",
  "policy_profile_id": "a3b2c1d0-1234-5678-9abc-def012345678",
  "enable_sandboxed_qa": true,
  "enable_load_test": false
}
```

**Response (202 Accepted):**
```json
{
  "audit_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "workflow_id": "wf-audit-f47ac10b",
  "status": "QUEUED",
  "created_at": "2026-08-25T21:40:00Z",
  "stream_url": "/api/v1/audits/f47ac10b-58cc-4372-a567-0e02b2c3d479/stream"
}
```

#### `GET /api/v1/audits/{audit_id}`
Fetches the current status, high-level summary, and links to generated reports.

#### `GET /api/v1/audits/{audit_id}/findings`
Lists all deduplicated findings with severity, confidence, proof state, and evidence references.

**Query Parameters:**
- `severity`: `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `INFO`
- `state`: `VERIFIED`, `SUSPECTED`, `RESOLVED`, `FALSE_POSITIVE`
- `category`: `SECURITY_VULN`, `LOGIC_BUG`, `SECRET_LEAK`, `PERFORMANCE`, `BREAKING_API`

---

### 2.2 Dynamic Target Authorization Management

#### `POST /api/v1/targets/verify`
Initiates a cryptographic challenge to prove ownership of a dynamic security/load testing target.

**Request Body:**
```json
{
  "project_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "base_url": "https://staging.api.example.com",
  "proof_method": "DNS_TXT" // "DNS_TXT" | "HTTP_WELL_KNOWN" | "OIDC_ROLE"
}
```

**Response (200 OK):**
```json
{
  "target_id": "tar-8839201",
  "challenge_type": "DNS_TXT",
  "record_name": "_codehound-challenge.staging.api.example.com",
  "expected_value": "ch-verify-9f8e7d6c5b4a3210",
  "expires_at": "2026-08-26T21:40:00Z",
  "status": "PENDING_VERIFICATION"
}
```

---

### 2.3 Verified Auto-Fix & Patch Management

#### `POST /api/v1/findings/{finding_id}/fix`
Triggers the multi-model patch generator and Firecracker sandbox re-test loop.

**Response (200 OK):**
```json
{
  "patch_id": "pat-102938",
  "finding_id": "fnd-48201",
  "status": "VERIFIED_PASSING",
  "mutation_score": 0.92,
  "unified_diff": "--- a/src/auth.ts\n+++ b/src/auth.ts\n@@ -42,7 +42,9 @@\n-  if (token == null) return false;\n+  if (!token || typeof token !== 'string') return false;\n",
  "pr_url": "https://github.com/org/repo/pull/142"
}
```

---

## 3. Server-Sent Events (SSE) Protocol (`/stream`)

Clients (CLI, Web Console, IDE LSP) subscribe to `/api/v1/audits/{audit_id}/stream` to receive real-time updates.

```text
event: stage_started
data: {"stage": "AST_PARSING", "timestamp": "2026-08-25T21:40:02Z"}

event: progress_update
data: {"stage": "AST_PARSING", "percent": 68, "files_indexed": 420, "symbols_extracted": 5120}

event: finding_discovered
data: {"finding_id": "fnd-901", "severity": "HIGH", "category": "SECRET_LEAK", "file": "config/db.py", "line": 14}

event: sandbox_test_executed
data: {"test_id": "tst-402", "framework": "pytest", "status": "PASSED", "duration_ms": 340, "mutation_score": 0.88}

event: audit_completed
data: {"audit_id": "f47ac10b", "health_score": 84, "report_uri": "/api/v1/audits/f47ac10b/report.md"}
```

---

## 4. Internal gRPC Worker Protocol (`sandbox.proto`)

```protobuf
syntax = "proto3";

package codehound.sandbox.v1;

service SandboxService {
  rpc ExecuteTask (ExecuteTaskRequest) returns (ExecuteTaskResponse);
  rpc StreamExecutionLogs (ExecuteTaskRequest) returns (stream LogChunk);
  rpc ValidatePatch (ValidatePatchRequest) returns (ValidatePatchResponse);
}

message ExecuteTaskRequest {
  string execution_id = 1;
  string repository_tar_uri = 2;
  string command = 3;
  repeated string environment_variables = 4;
  int32 timeout_seconds = 5;
  int32 max_memory_mb = 6;
  int32 cpu_count = 7;
  bool allow_network_egress = 8;
}

message ExecuteTaskResponse {
  string execution_id = 1;
  int32 exit_code = 2;
  int64 duration_ms = 3;
  string stdout = 4;
  string stderr = 5;
  string artifacts_tar_uri = 6;
}

message ValidatePatchRequest {
  string base_commit_sha = 1;
  string unified_diff = 2;
  string test_command = 3;
  bool run_mutation_tests = 4;
}

message ValidatePatchResponse {
  bool fix_confirmed = 1;
  bool no_regressions = 2;
  double mutation_score = 3;
  string test_output = 4;
}
```
