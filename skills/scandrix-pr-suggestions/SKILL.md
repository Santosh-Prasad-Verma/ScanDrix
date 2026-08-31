---
name: scandrix-pr-suggestions
description: Query and automatically apply actionable review suggestions on remote pull requests
---

# ScanDrix PR Suggestions Resolver

- **Fetch Suggestions**: Execute `scandrix pr suggestions` to retrieve current review findings for the active branch or PR.
- **Post Summary**: Execute `scandrix pr comment <pr_number> "Review Summary"` to publish a markdown audit report.
- **Business Rule Check**: Execute `scandrix pr business-validation` to verify compliance with task acceptance criteria.
