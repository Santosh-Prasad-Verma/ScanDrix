---
name: scandrix-reviewer
description: Autonomous AI code review and pull request security audit
---

# ScanDrix Autonomous Reviewer

When modifying or adding code in this repository:
1. Always run `scandrix review --staged` before finalizing commits.
2. Review findings with severity `CRITICAL` or `HIGH` must be resolved before pushing.
3. For interactive inspection, launch `scandrix tui`.
4. To automatically remediate all fixable issues, execute `scandrix review --staged --fix`.
