package review

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/internal/cli/types"
)

func TestValidateReviewOptions(t *testing.T) {
	// Interactive + PromptOnly conflict
	err := ValidateReviewOptions(ReviewOptions{
		Interactive: true,
		PromptOnly:  true,
	})
	if err == nil {
		t.Errorf("expected error for interactive + promptOnly")
	}

	// Interactive + Fix conflict
	err = ValidateReviewOptions(ReviewOptions{
		Interactive: true,
		Fix:         true,
	})
	if err == nil {
		t.Errorf("expected error for interactive + fix")
	}

	// Invalid fail-on
	err = ValidateReviewOptions(ReviewOptions{
		FailOn: "invalid-severity",
	})
	if err == nil {
		t.Errorf("expected error for invalid fail-on")
	}

	// Valid options
	err = ValidateReviewOptions(ReviewOptions{
		FailOn: "error",
		Staged: true,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildNoChangesMessages(t *testing.T) {
	msgs := BuildNoChangesMessages([]string{"main.go"}, false, "", "")
	if len(msgs) == 0 {
		t.Errorf("expected messages for explicit files")
	}

	msgs = BuildNoChangesMessages(nil, true, "", "")
	if len(msgs) == 0 || msgs[0] != "There are no staged changes to review." {
		t.Errorf("expected staged changes message, got %v", msgs)
	}

	msgs = BuildNoChangesMessages(nil, false, "feature/login", "")
	if len(msgs) == 0 {
		t.Errorf("expected branch message")
	}

	msgs = BuildNoChangesMessages(nil, false, "", "a1b2c3d")
	if len(msgs) == 0 {
		t.Errorf("expected commit message")
	}
}

func TestShouldFailReview(t *testing.T) {
	res := &types.ReviewResult{
		Issues: []types.ReviewIssue{
			{Severity: types.SeverityWarning, Message: "warn issue"},
			{Severity: types.SeverityError, Message: "error issue"},
		},
	}

	if !ShouldFailReview(res, "warning") {
		t.Errorf("expected fail on warning")
	}

	if !ShouldFailReview(res, "error") {
		t.Errorf("expected fail on error")
	}

	if ShouldFailReview(res, "critical") {
		t.Errorf("expected pass on critical")
	}

	msg := FormatFailOnExitMessage(res, "error")
	if msg == "" {
		t.Errorf("expected exit message for error")
	}
}

func TestConvertReviewToHunkContext(t *testing.T) {
	res := &types.ReviewResult{
		Issues: []types.ReviewIssue{
			{
				File:       "main.go",
				Line:       10,
				EndLine:    15,
				Severity:   types.SeverityError,
				Message:    "Potential nil pointer dereference. Ensure validation passes.",
				Suggestion: "if ptr == nil { return }",
				Category:   "correctness",
			},
		},
	}

	hunkContext := ConvertReviewToHunkAgentContext(res)
	if len(hunkContext.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(hunkContext.Files))
	}

	file := hunkContext.Files[0]
	if file.Path != "main.go" {
		t.Errorf("expected main.go, got %s", file.Path)
	}
	if len(file.Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(file.Annotations))
	}
	ann := file.Annotations[0]
	if ann.NewRange[0] != 10 || ann.NewRange[1] != 15 {
		t.Errorf("unexpected range %v", ann.NewRange)
	}

	sidecar := ConvertReviewToHunkFindings(res)
	if len(sidecar.Findings) != 1 {
		t.Fatalf("expected 1 sidecar finding, got %d", len(sidecar.Findings))
	}
	if sidecar.Findings[0].ID != "scandrix-0" {
		t.Errorf("expected scandrix-0 id, got %s", sidecar.Findings[0].ID)
	}
}

func TestHunkViewerScope(t *testing.T) {
	scope := BuildHunkViewerScope(ReviewOptions{
		Branch: "main",
		Files:  []string{"service.go"},
	})
	if scope == nil || scope.Range != "main...HEAD" {
		t.Errorf("unexpected scope %v", scope)
	}

	args := BuildHunkArgs(*scope, "/tmp/context.json", "")
	if len(args) == 0 || args[0] != "diff" {
		t.Errorf("unexpected hunk args: %v", args)
	}

	// Test ResolveReviewDiff
	ctx := context.Background()
	gitSvc := git.NewGitService("")
	diffRes, err := ResolveReviewDiff(ctx, ResolveReviewDiffParams{
		Options: ReviewOptions{Staged: true},
		Verbose: true,
		Git:     gitSvc,
	})
	if err != nil {
		t.Logf("ResolveReviewDiff: %v (expected if git repo is absent in test dir)", err)
	} else if diffRes == nil {
		t.Errorf("expected non-nil diff result")
	}
}

func TestBuildReviewPayloadConfigAndFilter(t *testing.T) {
	files := []types.FileContent{
		{Path: "small.go", Content: "package main", Diff: "+package main"},
		{Path: "huge.go", Content: string(make([]byte, MaxContentChars+100)), Diff: "+huge"},
	}

	filtered := FilterReviewFiles(files, true)
	if len(filtered) != 1 || filtered[0].Path != "small.go" {
		t.Errorf("expected huge file to be filtered out, got: %v", filtered)
	}

	cfg, err := BuildReviewPayloadConfig(BuildConfigOptions{
		RulesOnly: true,
		Fast:      true,
		Focus:     "security",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Fast || !cfg.RulesOnly || cfg.Focus != "security" {
		t.Errorf("unexpected payload config: %+v", cfg)
	}
}

func TestVerboseMessages(t *testing.T) {
	startMsgs := CreateAnalyzeStartVerboseMessages("diff content", false, true)
	if len(startMsgs) != 2 {
		t.Errorf("expected 2 start msgs, got %d", len(startMsgs))
	}

	apiMsgs := CreateAnalyzeAPIRequestVerboseMessages("diff", &ReviewPayloadConfig{Fast: true}, "team-key", "main", "origin")
	if len(apiMsgs) == 0 {
		t.Errorf("expected API request verbose messages")
	}

	trialMsgs := CreateTrialAnalyzeStartVerboseMessages("short diff")
	if len(trialMsgs) != 3 {
		t.Errorf("expected 3 trial messages, got %d", len(trialMsgs))
	}
}

func TestBuildReviewErrorHints(t *testing.T) {
	authHints := BuildReviewErrorHints(ReviewError{Code: "AUTH_REQUIRED"})
	if len(authHints) == 0 {
		t.Errorf("expected hints for AUTH_REQUIRED")
	}

	tooLargeHints := BuildReviewErrorHints(ReviewError{Code: "REVIEW_TOO_LARGE"})
	if len(tooLargeHints) == 0 {
		t.Errorf("expected hints for REVIEW_TOO_LARGE")
	}

	netHints := BuildReviewErrorHints(ReviewError{Code: "API_REQUEST_FAILED", Message: "Could not reach the ScanDrix API"})
	if len(netHints) == 0 {
		t.Errorf("expected hints for API reachability error")
	}
}
