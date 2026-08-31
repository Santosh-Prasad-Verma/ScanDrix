---
name: scandrix-business-rules-validation
description: Use when the user wants ScanDrix to validate local diff changes against task requirements, acceptance criteria, or business rules via `scandrix pr business-validation`, especially for implementation-vs-task or merge readiness checks.
license: MIT
metadata:
    author: ScanDrix
    version: '1.0'
---

# ScanDrix Business Rules Validation

## Goal

Run ScanDrix business-rules validation from the current repository diff against task requirements or issue tracker acceptance criteria (Jira, Linear, GitHub Issues).

## Required Inputs

- One local diff scope:
  - default working tree diff
  - or `--staged`
  - or `--branch <name>`
  - or `--commit <sha>`
  - or `[files...]`
- Optional task reference:
  - `--task-url <url>` or `--task-id <id>`
  - Do not pass both.

## When to Use

- The user asks for business validation, business rules validation, or acceptance criteria checks.
- The user wants to check local code implementation vs task requirements.
- The user mentions `scandrix pr business-validation`.

Do not use this skill as a substitute for `scandrix review`. Local code review (security/quality) and business-rules validation are separate flows.

## Workflow

1. **Choose the local scope**:
   - Default working tree diff when no scope flag is provided.
   - Use only one of `--staged`, `--branch`, `--commit`, or `[files...]`.

2. **Build and run the command**:

```bash
scandrix pr business-validation
scandrix pr business-validation --staged --task-id PROJ-102
scandrix pr business-validation --branch main --task-id PROJ-102
scandrix pr business-validation --commit HEAD~1 --task-id PROJ-102
scandrix pr business-validation internal/service.go --task-id PROJ-102
```

3. **Interpret the result**:
   - Output includes business rule compliance score (0.0 to 1.0) and missing criteria breakdown.
   - Address any gaps before requesting PR review.
