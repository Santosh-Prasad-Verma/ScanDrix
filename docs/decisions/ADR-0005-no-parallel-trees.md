# ADR-0005: Extend, Don't Duplicate (No Parallel Trees)

- **Status:** Accepted (enforced in review)
- **Context:** The first IMPLEMENTATION draft specified new files beside working ones (`verifier.go` vs validator/loader, `scim_service.go` vs `handler.go`, `dora_calculator.go` vs `dora_aggregator.go`, `multiagent/` vs `aiengine/`, migration `020` vs taken numbers). Builders following it would fork the codebase.
- **Decision:** EE work modifies existing packages; a NEW top-level path requires a one-paragraph justification citing why the existing package cannot carry it. Migration numbers continue from 030.
- **Consequences:** IMPLEMENTATION inventories are written as MODIFY-first; reviewers reject parallel-tree PRs by citing this ADR.
