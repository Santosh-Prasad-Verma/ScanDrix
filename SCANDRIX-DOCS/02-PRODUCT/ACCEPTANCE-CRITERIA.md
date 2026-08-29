# Scandrix — Enterprise Feature Acceptance Criteria & Gating

**Classification:** NORMATIVE QUALITY SPECIFICATION  
**Status:** APPROVED FOR IMPLEMENTATION  
**Version:** 3.0.0  
**Domain:** Product Verification & Automated Quality Gates

---

## 1. Fast Webhook Ingress & Ingestion Pipeline

### Scenario 1.1: Webhook Ingestion & Synchronous Acknowledgment
- **Given** an authenticated Git provider webhook delivery (GitHub, GitLab, Bitbucket, Azure Repos) carrying a valid cryptographic signature (`X-Hub-Signature-256` or `X-Gitlab-Token`),
- **When** the payload is posted to the endpoint `/api/v1/webhooks/{provider}`,
- **Then** the service must persist the payload to the PostgreSQL `outbox_events` table and return HTTP `202 Accepted` within $\le 15\text{ milliseconds}$,
- **And** the outbox relay daemon must publish the event to the RabbitMQ 3.13 Quorum Queue `q.jobs.scan.dag` with guaranteed at-least-once delivery.

### Scenario 1.2: Idempotent Delivery & Deduplication
- **Given** a duplicate webhook payload delivered multiple times by the Git provider due to network retry,
- **When** the worker processes the payload using the composite key `(tenant_id, provider, delivery_id, commit_sha)`,
- **Then** the worker must detect the existing record in the `inbox_events` table,
- **And** immediately acknowledge the message without triggering duplicate analysis runs or redundant bot comments.

---

## 2. Multi-Agent Investigative Review & Cognitive Budget

### Scenario 2.1: Parallel Agent Swarm Execution
- **Given** a pull request containing modified files across Go, TypeScript, and SQL,
- **When** the 18-stage analysis DAG reaches the agent review phase,
- **Then** the orchestrator must concurrently dispatch the four domain agents (`Bug & Logic`, `Security`, `Performance`, `Architecture`),
- **And** allow each agent to invoke sandboxed tools (`grep`, `readFile`, `astGrep`, `shell`) inside an isolated gVisor container,
- **And** collect and merge all candidate findings within an overall SLA of $\le 20\text{ seconds}$ (P95).

### Scenario 2.2: Cognitive Budget Capping (5 to 8 Recommendations)
- **Given** an analysis run that produces 25 raw candidate findings across all agents and scanners,
- **When** the Safeguard Filter evaluates the candidate pool,
- **Then** the filter must rank findings by the composite metric $\text{Severity} \times \text{Confidence}$,
- **And** post inline PR review comments for only the top **5 to 8 highest-priority actionable findings**,
- **And** consolidate the remaining lower-priority findings into an expandable markdown table inside the GitHub Check Run summary.

---

## 3. Change Stack & Automated Logic Sequence Diagrams

### Scenario 3.1: Change Stack Architectural Sorting
- **Given** a pull request containing schema migrations, business logic services, API route handlers, and React frontend components,
- **When** Scandrix generates the PR Walkthrough,
- **Then** it must present changes organized into four sequential layers:
  $$\text{Layer 1: Database Schema} \longrightarrow \text{Layer 2: Core Domain Logic} \longrightarrow \text{Layer 3: Ingress / API Handlers} \longrightarrow \text{Layer 4: UI Components}$$
- **And** include estimated review time (in minutes) and complexity rating (Low, Medium, High).

### Scenario 3.2: Automated Logic Sequence Diagram Generation
- **Given** a pull request modifying function interactions across two or more domain services,
- **When** the engine extracts the call graph using Tree-sitter and SCIP indexing,
- **Then** it must synthesize a valid Mermaid sequence diagram illustrating the runtime message flow,
- **And** verify that all node labels, edge annotations, and actors are safely double-quoted to guarantee zero syntax or parse errors.

---

## 4. Interactive `@scandrix` Slash Commands & Developer Chat

### Scenario 4.1: Automated Unit Test Synthesis (`@scandrix generate-tests`)
- **Given** a developer comments `@scandrix generate-tests` on an open pull request,
- **When** the command processor parses the request,
- **Then** Scandrix must inspect the modified functions, identify the repository's test framework (`testing` + `testify` for Go, `jest` / `vitest` for TypeScript, `pytest` for Python),
- **And** synthesize a complete, runnable test file containing edge-case and boundary-condition assertions,
- **And** post the test file as a formatted, committable code block in the PR discussion thread within $\le 10\text{ seconds}$.

### Scenario 4.2: Taint Path Explanation (`@scandrix explain --trace`)
- **Given** an inline security finding indicating an untrusted input sink,
- **When** a developer replies with `@scandrix explain --trace`,
- **Then** the bot must reply with a step-by-step call sequence diagram and line-by-line explanation showing how data flows from the public HTTP handler parameter down to the database query sink.

---

## 5. Issue Alignment & Project Management Verification

### Scenario 5.1: Scope & Acceptance Criteria Validation
- **Given** a pull request linked to a Jira or Linear issue (e.g., `PROJ-882`),
- **When** Scandrix ingests the issue description and acceptance criteria via API,
- **Then** it must compare the PR diff against the requirements,
- **And** annotate the PR with an **Issue Verification Checklist**:
  - `[PASS]` All acceptance criteria addressed in implementation.
  - `[WARNING]` Scope expansion: PR modifies unrelated authentication logic not specified in `PROJ-882`.

### Scenario 5.2: Automated Issue Resolution on Merge
- **Given** a pull request that passed all Scandrix assurance gates and was merged into the default branch,
- **When** the merge webhook is processed,
- **Then** Scandrix must transition the linked Jira/Linear issue to `In QA` or `Resolved`,
- **And** post an attestation comment containing the commit SHA, SLSA provenance link, and 6D Risk Vector score.

---

## 6. Closed-Loop Ephemeral Sandbox Proof-of-Fix

### Scenario 6.1: Sandboxed Compilation & Test Execution
- **Given** a confirmed vulnerability finding with an AI-synthesized candidate patch,
- **When** the Proof-of-Fix engine executes,
- **Then** it must spin up an ephemeral Firecracker MicroVM ($\le 120\text{ms}$ boot time),
- **And** apply the candidate patch to a clean repository worktree,
- **And** execute the project build command (`go build ./...`, `npm run build`),
- **And** execute existing unit tests (`go test -race ./...`).

### Scenario 6.2: Regression Test Synthesis & Clean Rescan
- **Given** a candidate patch that compiles cleanly and passes existing tests,
- **When** the Proof-of-Fix engine synthesizes a new regression test reproducing the original vulnerability,
- **Then** the engine must assert that:
  1. The new regression test passes against the patched codebase,
  2. A deterministic Tree-sitter rescan confirms the AST vulnerability pattern is completely eliminated,
- **And** emit a signed `ProofOfFixAttestation` enabling a one-click committable suggestion on the PR.

---

## 7. Multi-Tier Policy Engine & What-If Simulation

### Scenario 7.1: Hierarchical Policy Enforcement
- **Given** an active organization-level policy specifying `fail_on_severity: HIGH` and a repository-level `.scandrix/policy.yaml` specifying `profile: assertive`,
- **When** a pull request introduces a finding of severity `HIGH`,
- **Then** the policy engine must issue a `BLOCK` decision,
- **And** set the Git commit status / Check Run conclusion to `failure`.

### Scenario 7.2: What-If Historical Blast Radius Simulation
- **Given** a security administrator drafts a new custom AST rule or policy in the web cockpit,
- **When** the administrator clicks **Run What-If Simulation**,
- **Then** the engine must execute the rule against the last 500 merged pull requests in the repository history,
- **And** calculate the **Impact Ratio**:
  $$\text{Impact Ratio} = \frac{\text{PRs Blocked under New Policy}}{\text{500 Historical PRs}} \times 100\%$$
- **And** display a false-positive estimate and blast radius chart before policy activation.

---

## 8. Cryptographic Release Assurance & Kubernetes Admission

### Scenario 8.1: In-Toto v1.0 / SLSA Level 3 Manifest Generation
- **Given** a container image built from a merged release commit where all 18 assurance stages completed with `PASS` or `PASS_WITH_WARNINGS` and composite risk $R_{\text{composite}} \le 3.0$,
- **When** the release pipeline finalizes,
- **Then** Scandrix must generate an In-Toto v1.0 statement containing SHA-256 image digests, git commit provenance, and stage outcomes,
- **And** sign the statement with the platform's Ed25519 private key,
- **And** attach the attestation to the OCI registry via Sigstore Cosign.

### Scenario 8.2: Kyverno Admission Gating
- **Given** a production Kubernetes cluster configured with the Kyverno `check-scandrix-assurance-attestation` policy,
- **When** an engineer or CD pipeline attempts to deploy a container image,
- **Then** the Kyverno admission webhook must verify the Ed25519 signature against the trusted public key,
- **And** verify that the embedded risk vector meets namespace thresholds ($R_{\text{sec}} \le 3.0$),
- **And** reject admission with HTTP `403 Forbidden` if the manifest is missing, tampered with, or failing.
