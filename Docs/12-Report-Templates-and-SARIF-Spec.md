# Report Templates, SARIF & Artifact Deliverables — CodeHound (ForgeGuard)

**Document:** 12-Report-Templates-and-SARIF-Spec.md  
**Status:** Approved Specification  
**Target:** Master Audit Report (.md), SARIF v2.1.0, JUnit XML & CycloneDX SBOM  
**Date:** 2026-08-25  

---

## 1. Master Audit Markdown Report (`AUDIT_REPORT.md`)

Every completed scan compiles into a comprehensive, human-readable markdown artifact formatted as follows:

```markdown
# 🛡️ CodeHound Autonomous Audit & DevSecOps Report

**Repository:** `org/sample-backend`  
**Commit:** `7f83b2a` (`feat: add stripe checkout webhook`)  
**Audit ID:** `f47ac10b-58cc-4372-a567-0e02b2c3d479`  
**Date:** 2026-08-25 21:40:00 UTC  
**Overall Health Score:** `84 / 100`  

---

## 📊 Executive Summary & Posture Matrix

| Category | Status | Verified Issues | Suspected | Fixed in Branch |
| :--- | :---: | :---: | :---: | :---: |
| **Security & OWASP** | ⚠️ Warning | 2 Critical, 1 High | 3 Medium | 2 Verified Patches |
| **Logic & Reliability** | ⚠️ Warning | 1 Edge Case Bug | 2 Flaky Tests | 1 Verified Patch |
| **Secrets & Keys** | 🛑 Alert | 1 Live API Key | 0 | Auto-Masked |
| **Spike Resilience (50k VUs)** | ✅ Passed | 0 Bottlenecks | p95: 142ms | Target Healthy |
| **API Contract Integrity** | ✅ Clean | 0 Breaking Diffs | - | - |

---

## 🚨 Critical & High Verified Findings

### 1. [CRITICAL] SQL Injection via Unsanitized Tainted Route Parameter
- **Rule ID:** `OWASP-A03-SQLI` / `CWE-89`
- **File:** [`src/controllers/userController.ts:L48`](file:///src/controllers/userController.ts#L48)
- **Proof State:** `PROVEN_VERIFIED` (Generated test failed on source, passed on patch)
- **Taint Path:** `req.query.id` $\rightarrow$ `getUserDetails()` $\rightarrow$ `db.raw()`

#### Proof of Concept (Generated Sandboxed Test):
```typescript
it('fails when SQL injection payload is passed to user endpoint', async () => {
  const res = await request(app).get("/api/v1/user?id=1' OR '1'='1");
  expect(res.status).not.toBe(500); // Triggered unhandled database syntax error
});
```

#### Verified Auto-Patch:
```diff
- const user = await db.raw(`SELECT * FROM users WHERE id = '${req.query.id}'`);
+ const user = await db('users').where({ id: req.query.id }).first();
```

---

## ⚡ Load & Spike Resilience Analytics (1k $\rightarrow$ 10k $\rightarrow$ 50k VUs)

- **Peak Concurrency:** 50,000 Virtual Users
- **Peak Request Rate:** 18,400 RPS
- **Latency Distribution:** p50: `24ms` | p95: `142ms` | p99: `310ms`
- **Error Rate:** `0.02%` (No DB connection starvation or N+1 queries detected)
```

---

## 2. SARIF v2.1.0 Integration (GitHub Security Tab & IDEs)

CodeHound outputs standard SARIF so findings populate directly in GitHub Code Scanning:

```json
{
  "$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
  "version": "2.1.0",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "CodeHound Core",
          "version": "1.0.0",
          "rules": [
            {
              "id": "CH-SEC-0089",
              "shortDescription": { "text": "SQL Injection in User Query" },
              "defaultConfiguration": { "level": "error" }
            }
          ]
        }
      },
      "results": [
        {
          "ruleId": "CH-SEC-0089",
          "level": "error",
          "message": { "text": "Unsanitized user input passed directly to database query." },
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": { "uri": "src/controllers/userController.ts" },
                "region": { "startLine": 48, "endLine": 48 }
              }
            }
          ]
        }
      ]
    }
  ]
}
```
