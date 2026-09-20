// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package prompts_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/prompts"
)

func TestApplyFoundations_ResolvesAllPlaceholders(t *testing.T) {
	testTemplate := `
{{FOUNDATION_SCOPE_HIERARCHY}}
{{FOUNDATION_PROMPT_INJECTION_GUARDRAILS}}
{{FOUNDATION_EVIDENCE_GATE}}
{{FOUNDATION_CONFIDENCE_CALIBRATION}}
{{FOUNDATION_BLOCKING_POLICY}}
{{FOUNDATION_SIGNAL_TO_NOISE}}
{{FOUNDATION_ROOT_CAUSE_DEDUP}}
{{FOUNDATION_CONTEXT_DISCIPLINE}}
{{FOUNDATION_POSITIVE_SECURITY}}
{{FOUNDATION_REMEDIATION_SAFETY}}
{{FOUNDATION_QUALITY_GATE}}
{{CANONICAL_FINDING_SCHEMA}}
`

	result := prompts.ApplyFoundations(testTemplate)

	// Ensure no unexpanded placeholders remain
	unexpandedTokens := []string{
		"{{FOUNDATION_SCOPE_HIERARCHY}}",
		"{{FOUNDATION_PROMPT_INJECTION_GUARDRAILS}}",
		"{{FOUNDATION_EVIDENCE_GATE}}",
		"{{FOUNDATION_CONFIDENCE_CALIBRATION}}",
		"{{FOUNDATION_BLOCKING_POLICY}}",
		"{{FOUNDATION_SIGNAL_TO_NOISE}}",
		"{{FOUNDATION_ROOT_CAUSE_DEDUP}}",
		"{{FOUNDATION_CONTEXT_DISCIPLINE}}",
		"{{FOUNDATION_POSITIVE_SECURITY}}",
		"{{FOUNDATION_REMEDIATION_SAFETY}}",
		"{{FOUNDATION_QUALITY_GATE}}",
		"{{CANONICAL_FINDING_SCHEMA}}",
		"{{FOUNDATION_CANONICAL_SCHEMA}}",
		"{{SHARED_DELIBERATION_PREAMBLE}}",
	}

	for _, token := range unexpandedTokens {
		if strings.Contains(result, token) {
			t.Errorf("Expected token %s to be resolved, but found in output", token)
		}
	}
}

func TestApplyFoundations_NestedPreambleResolution(t *testing.T) {
	testTemplate := `{{SHARED_DELIBERATION_PREAMBLE}}`

	result := prompts.ApplyFoundations(testTemplate)

	// Verify that nested {{FOUNDATION_*}} in SharedDeliberationPreamble are resolved
	if strings.Contains(result, "{{FOUNDATION_") {
		t.Errorf("Nested foundation tokens remain in expanded preamble: %s", result)
	}

	// Verify key expected phrases
	expectedPhrases := []string{
		"SCOPE HIERARCHY",
		"PROMPT INTEGRITY & ANTI-INJECTION GUARDRAILS",
		"FINDING EVIDENCE GATE",
		"CONFIDENCE CALIBRATION",
		"BLOCKING POLICY",
		"SIGNAL-TO-NOISE RULE",
		"REMEDIATION SAFETY",
		"exploit_scenario",
		"root_cause",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(result, phrase) {
			t.Errorf("Expected expanded preamble to contain phrase %q", phrase)
		}
	}
}

func TestEvidenceGate_SevenCriteriaPresent(t *testing.T) {
	criteria := []string{
		"1. CODE EVIDENCE",
		"2. CAUSAL MECHANISM",
		"3. REALISTIC FAILURE / EXPLOIT PATH",
		"4. PRECONDITIONS",
		"5. MITIGATION CHECK",
		"6. CHANGE RELEVANCE",
		"7. ACTIONABILITY",
	}

	for _, c := range criteria {
		if !strings.Contains(prompts.FoundationEvidenceGate, c) {
			t.Errorf("FoundationEvidenceGate missing criterion: %s", c)
		}
	}
}

func TestQualityGate_FourteenChecklistItems(t *testing.T) {
	checklist := []string{
		"[ ] file_path exists in the supplied diff",
		"[ ] start_line and end_line exist in the supplied diff",
		"[ ] evidence matches the cited lines accurately",
		"[ ] root_cause is supported by evidence, not assumed",
		"[ ] severity matches the actual demonstrated impact",
		"[ ] confidence reflects genuine uncertainty level",
		"[ ] CWE ID is real and actually applicable",
		"[ ] OWASP mapping is actually applicable",
		"[ ] remediation addresses the root cause",
		"[ ] suggested_diff (if present) is safe and syntactically valid",
		"[ ] no duplicate finding with the same root cause exists",
		"[ ] existing_mitigations_checked shows genuine verification",
		"[ ] no claim depends on fabricated repository context",
		"[ ] blocking flag matches the Blocking Policy",
	}

	for _, item := range checklist {
		if !strings.Contains(prompts.FoundationQualityGate, item) {
			t.Errorf("FoundationQualityGate missing checklist item: %s", item)
		}
	}
}

func TestScopeHierarchy_PrecedenceLayers(t *testing.T) {
	layers := []string{
		"L6  SYSTEM RULES",
		"L5  ENTERPRISE SECURITY POLICY",
		"L4  REVIEW MODE",
		"L3  AGENT SPECIALIZATION",
		"L2  REPOSITORY DATA",
		"L1  USER CONTENT INSIDE REPOSITORY",
	}

	for _, layer := range layers {
		if !strings.Contains(prompts.FoundationScopeHierarchy, layer) {
			t.Errorf("FoundationScopeHierarchy missing layer: %s", layer)
		}
	}
}
