# Scandrix Enterprise Glossary & Architectural Lexicon

**Classification:** NORMATIVE ARCHITECTURAL SPECIFICATION  
**Status:** APPROVED  
**Target Version:** v1.0 Enterprise  
**Domain:** Enterprise Software Assurance & Risk Intelligence  

---

## 1. Core Domain Concepts

### Evidence Packet (`EvidencePacket`)
An immutable, cryptographically verifiable data record generated at any stage of the 18-Stage Assurance DAG. Every evidence packet contains an execution fingerprint, AST node reference, tool output hash, and an SHA-256 Merkle root. Stored in PostgreSQL with append-only triggers that reject `UPDATE` and `DELETE` queries.

### Proof-of-Fix (`ProofOfFix`)
A closed-loop remediation verification process. Rather than emitting speculative code suggestions, Scandrix synthesizes candidate diffs and executes them inside an isolated sandbox (gVisor or Firecracker). A fix is certified as a "Proof-of-Fix" if and only if the project builds cleanly, unit tests pass, no regression CVEs are introduced, and the original vulnerability is confirmed resolved.

### Security Twin (Code-to-Cloud Continuum)
A 7-hop topological digital twin that models the continuous path of code from local commit to cloud runtime:
$$\text{Commit} \to \text{Build/Artifact} \to \text{Registry} \to \text{IaC Deployment} \to \text{Cluster Ingress} \to \text{Service Mesh} \to \text{Runtime Pod}$$
Enables dynamic CVSS attenuation by verifying live compensating controls (e.g., active WAF rules, NetworkPolicies).

### 6D Continuous Risk Vector ($\vec{R}$)
A mathematical representation of software risk across six orthogonal engineering disciplines:
$$\vec{R} = [R_{\text{security}}, R_{\text{reliability}}, R_{\text{architecture}}, R_{\text{supply\_chain}}, R_{\text{performance}}, R_{\text{compliance}}]^T$$
Where each dimension $R_d \in [0.0, 100.0]$. Synthesized into a prioritized scalar composite score with an uncertainty penalty for incomplete scans.

### 18-Stage Continuous Assurance DAG
A directed acyclic graph of 18 parallel and sequential analysis stages executed upon pull request receipt. Stages span fast lexical triage, Tree-sitter AST extraction, taint tracking, reachability analysis, DAST, policy verification, and cryptographic attestation generation.

### Agent Firewall & Danger Tiers
A defense-in-depth authorization proxy governing all autonomous AI agent tool invocations:
- **Tier 1 (Read-Only / Safe)**: AST parsing, code grep, file viewing. Auto-allowed.
- **Tier 2 (Sandboxed Probe)**: Sandboxed linter execution (`go vet`, `npm test`). Auto-allowed within gVisor.
- **Tier 3 (External Read)**: CVE queries, package registry lookups. Restricted to approved outbound domains.
- **Tier 4 (Mutating Actions)**: Committing code, merging PRs, modifying cloud infrastructure. Strictly gated behind a cryptographically-signed human authorization token.

### What-If Policy Simulator
An offline regression testing engine that replays candidate security policies against historical pull requests (up to 500 merged PRs) to compute blast radius and developer friction before production policy activation.

### Cognitive Budget
An automated prioritization rule that limits high-visibility PR review comments to a configurable ceiling (default: 6 comments). Higher-severity and verified Proof-of-Fix items are prioritized; remaining observations are grouped into a collapsible summary to eliminate review fatigue.

### Security Memory
A vector-indexed organizational knowledge store backed by PostgreSQL `pgvector`. Captures developer review decisions, false-positive dismissals, and architectural justifications as 1536-dimensional embeddings. Automatically attenuates repetitive findings on matching code patterns.

### In-Toto Release Passport (SLSA Level 3)
A cryptographically signed Ed25519 provenance statement generated upon successful PR assurance verification. Validated by Kubernetes admission controllers (Kyverno / OPA) to prevent unsigned or high-risk container images from entering production clusters.

### Dual-Tier Sandbox Execution
A tiered isolation runtime separating fast and heavy execution workloads:
- **Tier 1 (gVisor `runsc`)**: User-space kernel interception with $<350\text{ms}$ startup for AST parsing and linters.
- **Tier 2 (Firecracker MicroVM)**: Dedicated Linux guest kernel via hardware KVM with $<80\text{ms}$ boot for executing untrusted customer test suites.
