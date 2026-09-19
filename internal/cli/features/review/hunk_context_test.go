// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package review

import (
	"testing"

	"github.com/scandrix/backend/internal/cli/types"
)

func TestConvertReviewToHunkAgentContext(t *testing.T) {
	result := &types.ReviewResult{
		Summary: "Found 3 issues across 2 files",
		Issues: []types.ReviewIssue{
			{
				File:     "pkg/auth/token.go",
				Line:     15,
				EndLine:  18,
				Severity: "error",
				Message:  "JWT token secret is hardcoded",
				RuleID:   "sec-jwt-hardcoded",
			},
			{
				File:     "pkg/auth/token.go",
				Line:     40,
				EndLine:  42,
				Severity: "warning",
				Message:  "Token expiration duration exceeds 24 hours",
				RuleID:   "sec-token-expiry",
			},
			{
				File:     "cmd/main.go",
				Line:     10,
				EndLine:  10,
				Severity: "info",
				Message:  "Consider using structured logging",
				RuleID:   "style-logging",
			},
		},
	}

	hunkCtx := ConvertReviewToHunkAgentContext(result)
	if hunkCtx == nil {
		t.Fatalf("expected non-nil HunkAgentContext")
	}

	if hunkCtx.Version != 1 {
		t.Errorf("expected version 1, got %d", hunkCtx.Version)
	}

	if len(hunkCtx.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(hunkCtx.Files))
	}

	// Files should be sorted alphabetically: cmd/main.go, then pkg/auth/token.go
	if hunkCtx.Files[0].Path != "cmd/main.go" {
		t.Errorf("expected cmd/main.go first, got %s", hunkCtx.Files[0].Path)
	}
	if len(hunkCtx.Files[0].Annotations) != 1 {
		t.Errorf("expected 1 annotation in cmd/main.go, got %d", len(hunkCtx.Files[0].Annotations))
	}

	if hunkCtx.Files[1].Path != "pkg/auth/token.go" {
		t.Errorf("expected pkg/auth/token.go second, got %s", hunkCtx.Files[1].Path)
	}
	if len(hunkCtx.Files[1].Annotations) != 2 {
		t.Errorf("expected 2 annotations in pkg/auth/token.go, got %d", len(hunkCtx.Files[1].Annotations))
	}

	// Annotations should be sorted by line
	anno1 := hunkCtx.Files[1].Annotations[0]
	if anno1.NewRange[0] != 15 || anno1.NewRange[1] != 18 {
		t.Errorf("expected range [15, 18], got %v", anno1.NewRange)
	}

	anno2 := hunkCtx.Files[1].Annotations[1]
	if anno2.NewRange[0] != 40 || anno2.NewRange[1] != 42 {
		t.Errorf("expected range [40, 42], got %v", anno2.NewRange)
	}
}

func TestConvertReviewToHunkAgentContext_NilResult(t *testing.T) {
	ctx := ConvertReviewToHunkAgentContext(nil)
	if ctx == nil || ctx.Version != 1 || len(ctx.Files) != 0 {
		t.Errorf("unexpected result for nil review: %+v", ctx)
	}
}

func TestConvertReviewToHunkAgentContext_SkipInvalidIssues(t *testing.T) {
	result := &types.ReviewResult{
		Issues: []types.ReviewIssue{
			{File: "", Line: 10, Message: "Missing file"},
			{File: "valid.go", Line: 0, Message: "Invalid line"},
			{File: "valid.go", Line: -5, Message: "Negative line"},
			{File: "valid.go", Line: 12, Message: "Valid issue"},
		},
	}

	hunkCtx := ConvertReviewToHunkAgentContext(result)
	if len(hunkCtx.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(hunkCtx.Files))
	}
	if len(hunkCtx.Files[0].Annotations) != 1 {
		t.Fatalf("expected 1 valid annotation, got %d", len(hunkCtx.Files[0].Annotations))
	}
	if hunkCtx.Files[0].Annotations[0].NewRange[0] != 12 {
		t.Errorf("expected line 12, got %d", hunkCtx.Files[0].Annotations[0].NewRange[0])
	}
}
