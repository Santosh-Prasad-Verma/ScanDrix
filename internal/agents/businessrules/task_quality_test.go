// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Tests
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeTaskQuality(t *testing.T) {
	cases := []struct {
		input    string
		expected TaskQuality
	}{
		{"empty", TaskQualityEmpty},
		{"MINIMAL", TaskQualityMinimal},
		{"partial", TaskQualityPartial},
		{"COMPLETE", TaskQualityComplete},
		{"unknown", TaskQualityEmpty},
		{"", TaskQualityEmpty},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeTaskQuality(tc.input)
			if got != tc.expected {
				t.Errorf("NormalizeTaskQuality(%q) = %v; want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestCanProceedWithBusinessRulesAnalysis(t *testing.T) {
	if CanProceedWithBusinessRulesAnalysis(TaskQualityEmpty) {
		t.Errorf("expected EMPTY to not proceed")
	}
	if CanProceedWithBusinessRulesAnalysis(TaskQualityMinimal) {
		t.Errorf("expected MINIMAL to not proceed")
	}
	if !CanProceedWithBusinessRulesAnalysis(TaskQualityPartial) {
		t.Errorf("expected PARTIAL to proceed")
	}
	if !CanProceedWithBusinessRulesAnalysis(TaskQualityComplete) {
		t.Errorf("expected COMPLETE to proceed")
	}
}

func TestClassifyTaskContextStatus(t *testing.T) {
	// 1. Empty
	if got := ClassifyTaskContextStatus(TaskQualityEmpty, "", nil); got != TaskContextStatusMissing {
		t.Errorf("expected missing, got %v", got)
	}

	// 2. Minimal
	if got := ClassifyTaskContextStatus(TaskQualityMinimal, "Fix bug", nil); got != TaskContextStatusWeak {
		t.Errorf("expected weak, got %v", got)
	}

	// 3. Structured metadata
	metadataJSON := `{"inlineCard": {"url": "https://jira.example.com", "attrs": {}}}`
	if got := ClassifyTaskContextStatus(TaskQualityPartial, metadataJSON, nil); got != TaskContextStatusWeak {
		t.Errorf("expected structured metadata to be classified as weak, got %v", got)
	}

	// 4. Task fetch failure
	failText := `{"error":true, "message":"Failed to fetch Jira task context: status: 404"}`
	if got := ClassifyTaskContextStatus(TaskQualityPartial, failText, nil); got != TaskContextStatusWeak {
		t.Errorf("expected fetch failure to be classified as weak, got %v", got)
	}

	// 5. Usable context
	validContext := "Title: User MFA Registration\nDescription: Allow users to register TOTP devices with backup codes."
	if got := ClassifyTaskContextStatus(TaskQualityPartial, validContext, nil); got != TaskContextStatusUsable {
		t.Errorf("expected usable context, got %v", got)
	}
}

func TestBuildBusinessLogicEligibility(t *testing.T) {
	// 1. Missing task context
	e1 := BuildBusinessLogicEligibility(TaskQualityEmpty, "", "+ diff", nil)
	if e1.Mode != EligibilityModeLimitationResponse || e1.Reason != "task_context_missing" {
		t.Errorf("unexpected eligibility: %+v", e1)
	}

	// 2. Weak task context
	e2 := BuildBusinessLogicEligibility(TaskQualityMinimal, "Fix it", "+ diff", nil)
	if e2.Mode != EligibilityModeLimitationResponse || e2.Reason != "task_context_weak" {
		t.Errorf("unexpected eligibility: %+v", e2)
	}

	// 3. Missing diff
	validContext := "Title: User MFA\nDescription: Detailed user requirement."
	e3 := BuildBusinessLogicEligibility(TaskQualityComplete, validContext, "", nil)
	if e3.Mode != EligibilityModeLimitationResponse || e3.Reason != "pr_diff_missing" {
		t.Errorf("unexpected eligibility: %+v", e3)
	}

	// 4. Fully analysis ready
	e4 := BuildBusinessLogicEligibility(TaskQualityComplete, validContext, "+ func MFA() {}", nil)
	if e4.Mode != EligibilityModeFullAnalysis || e4.Reason != "analysis_ready" {
		t.Errorf("expected full analysis ready, got: %+v", e4)
	}
}

func TestExtractTaskIdentifiers(t *testing.T) {
	text1 := "Resolves JIRA-1234 and links to https://linear.app/team/issue/ENG-987"
	text2 := "Closes #567 and fixes #890"

	keys := ExtractTaskIdentifiers(text1, text2)
	expected := []string{"JIRA-1234", "ENG-987", "#567", "#890"}

	for _, exp := range expected {
		found := false
		for _, k := range keys {
			if k == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected key %s to be extracted, got %v", exp, keys)
		}
	}
}

func TestLimitationMessages(t *testing.T) {
	msgEmpty := GetTaskContextMissingInfoMessage(TaskQualityEmpty)
	if !strings.Contains(msgEmpty, "Need Task Information") {
		t.Errorf("expected empty message header, got: %s", msgEmpty)
	}

	msgMinimal := GetTaskContextMissingInfoMessage(TaskQualityMinimal)
	if !strings.Contains(msgMinimal, "Insufficient Task Context") {
		t.Errorf("expected minimal message header, got: %s", msgMinimal)
	}

	msgDiff := GetPullRequestDiffMissingInfoMessage()
	if !strings.Contains(msgDiff, "Need Pull Request Diff") {
		t.Errorf("expected missing diff header, got: %s", msgDiff)
	}
}

func TestBlueprintPipeline_LimitationFastPath(t *testing.T) {
	ctx := context.Background()
	provider := NewBusinessRulesValidationAgentProvider(&mockAgentRunner{})
	pipeline := NewBlueprintPipeline(provider)

	// Test missing task context yields immediate limitation result
	res, err := pipeline.Run(ctx, BusinessRulesContext{
		TaskContext: "",
		PRDiff:      "+ some diff",
	})
	if err != nil {
		t.Fatalf("unexpected pipeline error: %v", err)
	}
	if !res.NeedsMoreInfo {
		t.Errorf("expected needsMoreInfo=true on missing task context")
	}
	if res.Reason != "task_context_missing" {
		t.Errorf("expected reason task_context_missing, got: %s", res.Reason)
	}
	if !strings.Contains(res.MissingInfo, "Need Task Information") {
		t.Errorf("expected missing info message, got: %s", res.MissingInfo)
	}
}
