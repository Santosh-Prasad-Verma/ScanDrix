// Package prompts implements code review safeguards, regression prevention, and verification prompts.
package prompts

import (
	"fmt"
	"strings"
)

// PromptCodeReviewSafeguard evaluates code diffs against critical production safeguards.
func PromptCodeReviewSafeguard(diff string, files []string) string {
	return fmt.Sprintf(`You are Drixy Safeguard Guardian. Review this pull request specifically for safety regressions:
1. Secret or credential leaks
2. Insecure deserialization / SQL injection / RCE
3. Accidental deletion of production database columns without migration safety
4. Memory or goroutine resource leaks
5. Broken backward compatibility in public API routes

## Files Changed
%s

## Diff
%s

Output JSON:
{
  "passed": true | false,
  "violations": [
    {
      "file": "path/to/file",
      "issue": "description of safety hazard",
      "severity": "critical"
    }
  ]
}`, strings.Join(files, "\n"), diff)
}

// PromptCodeReviewSafeguardFeatures analyzes feature-specific safety gates.
func PromptCodeReviewSafeguardFeatures(features []string, code string) string {
	return fmt.Sprintf(`Verify that code modifications adhere to enabled platform features:
Features: %s

Code:
%s

Output JSON: {"isCompliant": true, "notes": "all features respected"}`, strings.Join(features, ", "), code)
}

// PromptCodeReviewSafeguardVerification checks if an AI suggestion is actually safe and does not introduce new bugs.
func PromptCodeReviewSafeguardVerification(suggestion, rule string) string {
	return fmt.Sprintf(`You are Drixy Quality Verifier. Verify whether this proposed code change is 100%% safe:
Rule: %s

Proposed Suggestion:
%s

Output JSON:
{
  "isSafe": true | false,
  "confidenceScore": 0.95,
  "verificationVerdict": "approve | discard"
}`, rule, suggestion)
}
