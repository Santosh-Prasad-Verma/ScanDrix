# Evaluation Methodology (Golden Corpus + Gates)

This doc makes the PRD §6 scorecard measurable. No quality number (68% acceptance, <4% FP, >94% CWE catch, 0.92 threshold) may appear in releases or customer material until its gate below passes with the corpus version recorded alongside the number.

## Status

`evals/` today covers review *shape* (anchoring, dedup, severity, format, parser, investigation, scorer, promotion, summaries, tracecontext) — necessary but insufficient for accuracy claims. Precision/recall ground truth on a frozen golden corpus is **SPECCED** (IMPLEMENTATION gate 5) and is the missing piece.

## 1. Golden corpus

- **Composition (minimum viable):** 300+ real PRs across Go/TypeScript/Python, stratified by category (feature, bug, security, refactor) and size (small/medium/large diffs, capped at the 50-file/2500-line NFR envelope). Each PR carries ground-truth labels: true findings (file, line-range, CWE/category, severity) adjudicated by two senior reviewers; disagreements resolved by a third, with agreement rate published.
- **Security subset:** ≥80 PRs with known CVE/CWE instances (vulnerability catch rate is computed here only).
- **Versioning:** corpus is immutable per version (`corpus-v<date>-<n>`); additions create new versions, never mutate old ones. Every reported metric cites its corpus version.
- **Refresh:** quarterly; drift (stale frameworks, new CWEs) tracked as corpus issues, not silently patched.

## 2. Metrics (definitions, frozen)

| Metric | Numerator / denominator | Gate |
|---|---|---|
| Precision (FP rate = 1 − precision) | Posted findings matching a ground-truth label (same file ±5 lines, same category) / all posted findings | FP < 4% on current corpus version |
| Recall (CWE catch) | Ground-truth security labels matched / all security labels | > 94% on security subset |
| Acceptance rate | Suggestions merged without edit (measured from SCM merge data, not self-reported) / posted suggestions | > 68% over 90-day customer aggregate, min 500 suggestions |
| Latency P50/P90 | Worker `review.duration_seconds` histogram, PRs within NFR envelope | P50 < 20s, P90 < 45s over 7-day window |
| Verification coverage | Findings posted `verified` / all posted code-change findings | Reported always; target set after 2 quarters of data (no invented target) |

Confidence scores (0.92 operating point) are LLM self-scores: the gate freezes the *threshold value*, never certifies it as a probability. Recalibration runs with each model change.

## 3. CI wiring

- `go test ./evals/...` runs shape suites per PR; golden precision/recall runs nightly + on model/prompt changes (expensive by design — quarantined from per-PR CI).
- Threshold, model IDs, and corpus version are recorded with every result; regressions beyond ±2pp block promotion.
- Human spot-audit: 5% of nightly runs re-labeled blind; labeler agreement <90% invalidates the run, not the code.

## 4. What may be claimed, when

| Stage | Allowed claim |
|---|---|
| Before corpus v1 | "Targets under evaluation; methodology in `docs/evaluation/methodology.md`" — no numbers |
| After gate passes | Numbers **with** corpus version, date, and metric definition link |
| Customer aggregate (acceptance) | Only with stated sample size + window; never extrapolated to "all customers" |

Fabricating, back-filling, or cherry-picking eval numbers is a release-blocking violation, not a marketing judgment call.
