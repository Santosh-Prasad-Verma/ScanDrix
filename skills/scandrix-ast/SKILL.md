---
name: scandrix-ast
description: AST complexity analysis, cyclomatic depth checks, and anti-pattern detection
---

# ScanDrix AST Intelligence

- **Cyclomatic Complexity**: Functions must maintain cyclomatic complexity ≤ 15.
- **Nesting Depth**: Block nesting depth must not exceed 4 levels.
- **Modularity**: Break down large modules (> 500 lines) into cohesive domain submodules.
- **Clean Contracts**: Expose clear interface boundaries and avoid leaky abstractions.
