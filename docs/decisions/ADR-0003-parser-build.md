# ADR-0003: Tree-sitter Requires a Build Decision (CGO vs Pure-Go)

- **Status:** Open (blocks Epic 3) — decision due at Phase 2 kickoff
- **Context:** Epic 3 assumes Tree-sitter CGO bindings, but `go.mod` has no tree-sitter dependency, no CGO exists in the tree, and the production `Dockerfile` builds with `CGO_ENABLED=0`.
- **Options:** (a) enable CGO in the builder with an `amd64/arm64` matrix + vendored grammars; (b) pure-Go parsers per language.
- **Acceptance (either path):** ≥99% parse success on files <500KB (golden corpus); failures degrade to labeled diff-only context.
- **Consequences:** Until decided, Epic 3 dates are targets, not commitments; no grammar work starts without the recorded matrix.
