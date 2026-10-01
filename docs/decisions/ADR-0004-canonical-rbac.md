# ADR-0004: Single Canonical RBAC Matrix

- **Status:** Accepted (migration staged)
- **Context:** Three role systems coexist: `enterprise/rbac` (`OWNER/ADMIN/MEMBER/VIEWER`), `identity` (8 roles), `auth.RoleGuard`. PRD once named a fourth set. Parallel matrices caused the SSO-config/audit route confusion.
- **Decision:** `enterprise/rbac/policy.go` is canonical. Enterprise display titles map onto it (PRD REQ-5.3 table); no new role system may be added. `identity` and `RoleGuard` converge via adapter shims (REQ-8.8).
- **Consequences:** Every new permission check cites the canonical matrix; reviews reject new role tables.
