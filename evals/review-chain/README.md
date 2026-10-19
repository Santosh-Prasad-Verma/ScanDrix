# Review-Chain Evaluation & Resilience Matrix

> - **Answers:** Is every LLM call in the ScanDrix review orchestrator pipeline protected with fail-safe structured output recovery and envelope normalization?
> - **Runner:** `go test -v ./evals/...`
> - **Engine:** ScanDrix native Go review pipeline (`internal/review/orchestrator/`).

## Overview

The ScanDrix multi-agent review pipeline validates all LLM model outputs through strict JSON schema validation, envelope recovery, and Markdown extraction. This ensures that arbitrary model outputs (thinking blocks, fenced JSON, renamed fields) are normalized without silently dropping review findings.

## Evaluated Pipeline Stages

1. **Triage & File Classification** — AST relevance scoring and diff partitioning.
2. **Specialist Multi-Agent Deliberation** — Security, bug detection, architecture, and business logic agents.
3. **Judge & Consensus Arbiter** — De-duplication and confidence filtering.
4. **Attestation & Output Sanitization** — SLSA provenance and line mapping.
