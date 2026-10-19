# Reply-Addressing Evaluation

> - **Answers:** When someone replies in a review thread started by ScanDrix (Drixy) without explicitly tagging `@scandrix`, does the classifier correctly discern replies intended for the AI versus human-to-human discussion?
> - **Runner:** `go test -v ./evals/...`
> - **Architecture:** Implemented in `internal/review/conversation/` in native Go.

## Overview

Evaluates precision, recall, and specificity of conversational intent classification on PR threads. Prevents the AI reviewer from interrupting conversations between human engineers while remaining responsive when developers reply to review findings.

## Datasets

- `cases.json` — Edge cases and multi-developer conversation threads.
- `repo-cases.json` — Verified human responses and dispositions to review comments.
