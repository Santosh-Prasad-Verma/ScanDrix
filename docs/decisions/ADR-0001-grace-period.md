# ADR-0001: 7-Day License Grace Period

- **Status:** Accepted
- **Context:** PRD REQ-7.3 and WORKFLOWS §6 specified 7 days; `LicenseGracePeriod` was 72h.
- **Decision:** 7 days (`7*24h`). Rationale: enterprise procurement/renewal cycles stall on weekends; 72h converts admin delays into production lockouts, while 7 days preserves enforcement (expired + grace = hard gate) with warnings throughout.
- **Consequences:** `LoadLicense` and `EntitlementFromLicense` share the constant — single change point. Clock-skew tolerance needs no separate mechanism (minutes of skew are noise against 7 days).
