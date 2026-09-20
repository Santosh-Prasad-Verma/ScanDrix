---
name: drixy-reviewer
description: Autonomous AI code review and developer companion (Drixy) for ScanDrix. Use to trigger pull request reviews, interactive chat audits, and manage .drixy/rules/.
---

# Drixy — Autonomous AI Code Reviewer

**Drixy** (⚡ `drixy[bot]`) is the primary autonomous AI code reviewer and security companion for ScanDrix.

## 1. Invoking Drixy

### Pull Request Comments & Discussions
Mention Drixy directly in any GitHub/GitLab PR thread or review discussion:
- `@drixy review` — Trigger a full agentic code review of the pull request diff.
- `@drixy -v business-logic` — Trigger deep domain and business-logic verification.
- `@drixy` — Ask questions, request explanation of AST findings, or ask for remediation diffs.
- `[drixy-skip]` or `/drixy-rules` — Include in commit messages or PR descriptions to control review execution or discover rules.

### CLI & Interactive Terminal
- `scandrix chat` — Start an interactive conversation with Drixy directly in your terminal.
- `scandrix review --staged` — Run an autonomous AST & AI diff review on staged git changes.
- `scandrix tui` — Open the terminal UI dashboard to inspect findings and review thoughts.

## 2. Managing Drixy Rules (`.drixy/rules/`)

Drixy enforces both organization rules and repository-level policies:
- **Starter Rules Config:** `.drixy/rules.yaml`
- **Markdown Domain Policies:** `.drixy/rules/**/*.md` and `.drixy/rules/*.md`
- **CLI Commands:**
  - `scandrix rules init` — Scaffold `.drixy/rules.yaml` starter policy.
  - `scandrix rules validate` — Verify regex syntax and severity levels.
  - `scandrix rules sync` — Synchronize local rules with organization database policies.
  - `scandrix rules list` — List all active AST and regex rules.

## 3. Feedback & Sentiment Memory
Drixy incorporates reinforcement learning from developer sentiment:
- React with 👍 or 👎 on any inline review comment posted by Drixy.
- Drixy calculates workspace helpfulness and false-positive rates to self-tune precision and minimize noise in future reviews.
