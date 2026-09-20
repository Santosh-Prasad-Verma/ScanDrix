// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package operations

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

func TestIssueSyncService_TrackingAndFingerprints(t *testing.T) {
	svc := NewIssueSyncService()

	fp := ComputeFingerprint("SEC-001", "internal/auth/token.go", 42)
	if fp != "SEC-001:internal/auth/token.go:42" {
		t.Fatalf("unexpected fingerprint: %s", fp)
	}

	finding := TrackedFinding{
		RuleID:      "SEC-001",
		FilePath:    "internal/auth/token.go",
		Line:        42,
		Severity:    "critical",
		Title:       "Potential Hardcoded Secret",
		Description: "Detected high-entropy string in token initialization",
	}

	svc.TrackFinding(finding)

	retrieved, ok := svc.GetTrackedFinding(fp)
	if !ok || retrieved.RuleID != "SEC-001" || retrieved.Status != "open" {
		t.Fatalf("unexpected tracked finding: %+v", retrieved)
	}

	list := svc.ListTrackedFindings("open")
	if len(list) != 1 || list[0].Fingerprint != fp {
		t.Fatalf("unexpected list of findings: %+v", list)
	}

	payload, err := svc.BuildProviderIssuePayload(models.ProviderGitHub, retrieved)
	if err != nil {
		t.Fatalf("BuildProviderIssuePayload error: %v", err)
	}
	if !strings.Contains(payload.Body, "ScanDrix Automated Security Finding") || !strings.Contains(payload.Body, fp) {
		t.Fatalf("unexpected payload body: %s", payload.Body)
	}
}

func TestIssueSyncService_ResolutionTracking(t *testing.T) {
	svc := NewIssueSyncService()

	f1 := TrackedFinding{RuleID: "RULE-1", FilePath: "main.go", Line: 10, Status: "open"}
	f2 := TrackedFinding{RuleID: "RULE-2", FilePath: "main.go", Line: 20, Status: "open"}
	svc.TrackFinding(f1)
	svc.TrackFinding(f2)

	// New revision: RULE-1 remains, RULE-2 is fixed (absent), and RULE-3 is new
	f3 := TrackedFinding{RuleID: "RULE-3", FilePath: "util.go", Line: 5}
	report := svc.CheckFindingResolution([]TrackedFinding{f1, f3})

	if len(report.ResolvedFindings) != 1 || report.ResolvedFindings[0].RuleID != "RULE-2" {
		t.Fatalf("expected RULE-2 to be resolved, got %+v", report.ResolvedFindings)
	}
	if len(report.RemainingFindings) != 1 || report.RemainingFindings[0].RuleID != "RULE-1" {
		t.Fatalf("expected RULE-1 to remain, got %+v", report.RemainingFindings)
	}
	if len(report.NewFindings) != 1 || report.NewFindings[0].RuleID != "RULE-3" {
		t.Fatalf("expected RULE-3 to be new, got %+v", report.NewFindings)
	}
}

func TestIssueSyncService_AggregateReactions(t *testing.T) {
	svc := NewIssueSyncService()

	comments := []ReviewCommentReference{
		{
			CommentID: "c-101",
			Provider:  models.ProviderGitHub,
			Reactions: []string{"+1", "thumbsup", "heart", "confused"},
		},
		{
			CommentID: "c-102",
			Provider:  models.ProviderGitLab,
			Reactions: []string{"-1", "dislike"},
		},
	}

	aggregates := svc.AggregateReactions(comments)
	if len(aggregates) != 2 {
		t.Fatalf("expected 2 aggregates, got %d", len(aggregates))
	}

	c101 := aggregates["c-101"]
	if c101.TotalCount != 4 || !c101.HasPositive || !c101.HasNegative || c101.Reactions["+1"] != 2 {
		t.Fatalf("unexpected c-101 reactions: %+v", c101)
	}

	c102 := aggregates["c-102"]
	if c102.TotalCount != 2 || c102.HasPositive || !c102.HasNegative || c102.Reactions["-1"] != 2 {
		t.Fatalf("unexpected c-102 reactions: %+v", c102)
	}
}
