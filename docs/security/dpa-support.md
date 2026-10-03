# DPA & Compliance Support Pack

For procurement and legal review. This pack supports the order-form DPA; it does not replace legal counsel.

## 1. Processing summary

- **Roles:** customer = controller, ScanDrix (SaaS) = processor. Self-hosted/air-gapped: customer processes locally; ScanDrix has no processor role for code content.
- **Subject matter:** source-code diffs and repository metadata, strictly to provide code review + security findings.
- **Data categories:** code, commit metadata (authors, emails), PR discussions, account identities. No special-category data is requested; customers must not send it.
- **Retention:** per the retention schedule (REQ-8.2) + zero-retention mode; deletion on termination within 30 days with written confirmation.

## 2. Region & residency

Processing region pinned per order (SaaS); self-hosted/air-gapped process in-customer-perimeter by construction. Cross-border transfers use SCCs where applicable; transfer impact assessment available on request.

## 3. Breach notification

Confirmed personal-data breach: notice to affected customers within 72 hours of confirmation, with nature, categories, approximate subjects, consequences, and mitigations. Suspected-but-unconfirmed events follow `operations/runbooks.md` §4 first — notification clocks start at confirmation, and the distinction is documented in the incident record.

## 4. Data-subject requests

Access/erasure/portability requests: SaaS fulfilled within 30 days via support channel with identity verification; self-hosted fulfilled by the customer using the retention/purge tooling (assisted on request).

## 5. Audit rights

Annual customer audit (or auditor) with 30 days' notice, scoped to processor controls; continuous assurance via the quarterly egress report + pen-test summary under NDA. Findings both sides track to closure with dates.
