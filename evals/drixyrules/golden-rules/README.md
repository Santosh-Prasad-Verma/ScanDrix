# Drixy Rules Adherence Evaluation

> - **Answers:** Does the Drixy Rules agent flag every place an enterprise rule is violated?
> - **Runner:** `go test -v ./evals/drixyrules/...`
> - **Gate:** Uses the per-model accuracy thresholds in `evals/drixyrules/golden-rules/drixy-targets.json`.
> - **Engine:** Executes ScanDrix native Go multi-agent review and AST validation engine.

## Overview

Validates that the ScanDrix review engine accurately detects rule violations defined in `.drixy/rules.yaml` across pull requests.

## Metrics Evaluated

- **Occurrence Recall** — When a rule is violated multiple times across modified files, what percentage of actual sites are caught within line tolerance (±2 lines).
- **Line Precision** — Of the flagged violations emitted, what percentage land directly on real violating lines rather than pre-existing surrounding context.
- **Specificity** — Of clean files in the PR diff, ensure zero false-positive flags are emitted.

## Ground-Truth Datasets

- **`polyglot-cases-with-content.json`** — Golden benchmark cases across TypeScript, Go, Python, Java, Ruby, and Rust.
- **`github-cases.json` & `github-cases-v2.json`** — Frozen real-world PR diffs harvested from open-source repositories to ensure deterministic CI runs.
- **`rails-convention-*.json`** — Framework-specific architecture rules and convention adherence test cases.
