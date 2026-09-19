// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

func TestDrixyRuleGenerator_Empty(t *testing.T) {
	gen := NewDrixyRuleGenerator()
	rules, err := gen.GenerateRules(context.Background(), RuleSynthesisRequest{
		OrgID:              "org-1",
		RepoID:             "repo-1",
		MinClusterSize:     2,
		HistoricalComments: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(rules))
	}
}

func TestDrixyRuleGenerator_SynthesisAndFingerprint(t *testing.T) {
	gen := NewDrixyRuleGenerator()

	comments := []PastReviewComment{
		{
			ID:          "c1",
			PRNumber:    101,
			Author:      "alice",
			FilePath:    "pkg/db/user_repo.go",
			LineNumber:  45,
			Body:        "Please do not use fmt.Sprintf with raw query, this is a sql injection risk!",
			CreatedAt:   time.Now().Add(-48 * time.Hour),
		},
		{
			ID:          "c2",
			PRNumber:    102,
			Author:      "bob",
			FilePath:    "pkg/db/order_repo.go",
			LineNumber:  82,
			Body:        "Potential raw query here, format string could lead to sql injection.",
			CreatedAt:   time.Now().Add(-24 * time.Hour),
		},
		{
			ID:          "c3",
			PRNumber:    103,
			Author:      "charlie",
			FilePath:    "pkg/api/auth.go",
			LineNumber:  12,
			Body:        "Hardcoded api key found, never commit tokens in source code.",
			CreatedAt:   time.Now().Add(-12 * time.Hour),
		},
		{
			ID:          "c4",
			PRNumber:    104,
			Author:      "alice",
			FilePath:    "pkg/api/client.go",
			LineNumber:  19,
			Body:        "Hardcoded secret key in the client initialization!",
			CreatedAt:   time.Now().Add(-6 * time.Hour),
		},
		{
			ID:          "c5",
			PRNumber:    105,
			Author:      "dave",
			FilePath:    "pkg/util/misc.go",
			LineNumber:  5,
			Body:        "One-off naming suggestion.",
			CreatedAt:   time.Now(),
		},
	}

	rules, err := gen.GenerateRules(context.Background(), RuleSynthesisRequest{
		OrgID:              "org-alpha",
		RepoID:             "repo-beta",
		MinClusterSize:     2,
		HistoricalComments: comments,
	})
	if err != nil {
		t.Fatalf("failed to generate rules: %v", err)
	}

	// Should have 2 synthesized rules: SQL injection and Hardcoded secret (naming has support count 1 < 2)
	if len(rules) != 2 {
		t.Fatalf("expected 2 synthesized rules, got %d", len(rules))
	}

	sqlRule := rules[0]
	if sqlRule.SupportCount != 2 {
		t.Errorf("expected support count 2, got %d", sqlRule.SupportCount)
	}
	if sqlRule.Severity != models.SeverityCritical {
		t.Errorf("expected critical severity for sql injection, got %v", sqlRule.Severity)
	}
	if sqlRule.DetectorPattern == "" {
		t.Errorf("expected non-empty detector pattern")
	}
	if sqlRule.NegativePattern == "" {
		t.Errorf("expected non-empty negative pattern")
	}
	if sqlRule.CorrectExample == "" || sqlRule.IncorrectExample == "" {
		t.Errorf("expected examples to be populated")
	}

	// Verify deterministic fingerprint
	fp1 := ComputeRuleFingerprint(&sqlRule)
	fp2 := ComputeRuleFingerprint(&sqlRule)
	if fp1 != fp2 || len(fp1) != 64 {
		t.Errorf("fingerprint mismatch or invalid length: %s vs %s", fp1, fp2)
	}
}

func TestRuleConflictDetector_Contradiction(t *testing.T) {
	detector := NewRuleConflictDetector()

	parentA := uuid.New()
	parentB := uuid.New()

	ruleA := &DrixyRuleAtom{
		ID:             "atom-1",
		ParentRuleID:   parentA,
		Title:          "Enforce Context Propagation",
		Spec:           "All external calls must pass context.Context",
		NormativeLevel: NormativeMust,
		Severity:       models.SeverityHigh,
		PathGlobs:      []string{"**/*.go"},
		Detector: &CompiledRuleDetector{
			Pattern: `(?i)http\.get\(`,
		},
	}

	ruleB := &DrixyRuleAtom{
		ID:             "atom-2",
		ParentRuleID:   parentB,
		Title:          "Enforce Context Propagation",
		Spec:           "Calls must not require context parameter",
		NormativeLevel: NormativeMustNot,
		Severity:       models.SeverityHigh,
		PathGlobs:      []string{"**/*.go"},
		Detector: &CompiledRuleDetector{
			Pattern: `(?i)http\.get\(`,
		},
	}

	report := detector.InspectRuleCollection([]*DrixyRuleAtom{ruleA, ruleB})

	if report.TotalConflicts != 1 {
		t.Fatalf("expected 1 conflict, got %d", report.TotalConflicts)
	}
	if report.CriticalCount != 1 {
		t.Errorf("expected 1 critical conflict, got %d", report.CriticalCount)
	}
	if report.Conflicts[0].Kind != KindContradiction {
		t.Errorf("expected CONTRADICTION kind, got %s", report.Conflicts[0].Kind)
	}
}

func TestRuleConflictDetector_RedundancyAndShadowing(t *testing.T) {
	detector := NewRuleConflictDetector()

	// Redundant rules
	rule1 := &DrixyRuleAtom{
		ID:             "atom-1",
		ParentRuleID:   uuid.New(),
		Title:          "No Sprintf in Queries",
		NormativeLevel: NormativeMustNot,
		Severity:       models.SeverityCritical,
		PathGlobs:      []string{"**/*.go"},
		Detector: &CompiledRuleDetector{
			Pattern: `fmt\.Sprintf\(.*SELECT`,
		},
	}

	rule2 := &DrixyRuleAtom{
		ID:             "atom-2",
		ParentRuleID:   uuid.New(),
		Title:          "Avoid Sprintf In Database",
		NormativeLevel: NormativeMustNot,
		Severity:       models.SeverityCritical,
		PathGlobs:      []string{"**/*.go"},
		Detector: &CompiledRuleDetector{
			Pattern: `fmt\.Sprintf\(.*SELECT`,
		},
	}

	reportRedundant := detector.InspectRuleCollection([]*DrixyRuleAtom{rule1, rule2})
	if reportRedundant.TotalConflicts != 1 || reportRedundant.InfoCount != 1 {
		t.Errorf("expected 1 redundancy conflict, got %d (info: %d)", reportRedundant.TotalConflicts, reportRedundant.InfoCount)
	}

	// Shadowing / severity mismatch with same title
	rule3 := &DrixyRuleAtom{
		ID:             "atom-3",
		ParentRuleID:   uuid.New(),
		Title:          "Standard Log Format",
		NormativeLevel: NormativeShould,
		Severity:       models.SeverityLow,
		PathGlobs:      []string{"**/*.go"},
		Detector: &CompiledRuleDetector{
			Pattern: `log\.Print`,
		},
	}

	rule4 := &DrixyRuleAtom{
		ID:             "atom-4",
		ParentRuleID:   uuid.New(),
		Title:          "Standard Log Format",
		NormativeLevel: NormativeShould,
		Severity:       models.SeverityHigh,
		PathGlobs:      []string{"**/*.go"},
		Detector: &CompiledRuleDetector{
			Pattern: `fmt\.Print`,
		},
	}

	reportShadow := detector.InspectRuleCollection([]*DrixyRuleAtom{rule3, rule4})
	if reportShadow.TotalConflicts != 1 || reportShadow.WarningCount != 1 {
		t.Errorf("expected 1 shadowing conflict, got %d (warn: %d)", reportShadow.TotalConflicts, reportShadow.WarningCount)
	}

	// Disjoint paths: No overlap between .go and .py
	ruleGo := &DrixyRuleAtom{
		ID:             "atom-go",
		ParentRuleID:   uuid.New(),
		Title:          "Identical Pattern Different Langs",
		NormativeLevel: NormativeMustNot,
		Severity:       models.SeverityHigh,
		PathGlobs:      []string{"**/*.go"},
		Detector: &CompiledRuleDetector{
			Pattern: `(?i)secret_token`,
		},
	}
	rulePy := &DrixyRuleAtom{
		ID:             "atom-py",
		ParentRuleID:   uuid.New(),
		Title:          "Identical Pattern Different Langs",
		NormativeLevel: NormativeMustNot,
		Severity:       models.SeverityHigh,
		PathGlobs:      []string{"**/*.py"},
		Detector: &CompiledRuleDetector{
			Pattern: `(?i)secret_token`,
		},
	}

	reportDisjoint := detector.InspectRuleCollection([]*DrixyRuleAtom{ruleGo, rulePy})
	if reportDisjoint.TotalConflicts != 0 {
		t.Errorf("expected 0 conflicts for disjoint path globs, got %d", reportDisjoint.TotalConflicts)
	}
}

func TestRuleHealthOptimizer_EvaluationAndNegativeSynthesis(t *testing.T) {
	optimizer := NewRuleHealthOptimizer(0.35) // 35% toxic threshold
	ruleID := uuid.New()

	// 1. Excellent Rule
	feedbacksExcellent := []DeveloperRuleFeedback{
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
		{RuleID: ruleID, FeedbackType: "THUMBS_UP"},
		{RuleID: ruleID, FeedbackType: "THUMBS_UP"},
	}
	reportExc := optimizer.EvaluateRuleHealth(context.Background(), ruleID, feedbacksExcellent, nil)
	if reportExc.Grade != HealthExcellent {
		t.Errorf("expected HealthExcellent, got %s", reportExc.Grade)
	}

	// 2. Toxic Rule (>35% false positive with >=5 decisions)
	feedbacksToxic := []DeveloperRuleFeedback{
		{RuleID: ruleID, FeedbackType: "FALSE_POSITIVE", CodeSnippet: "mockClient.ExecuteQuery()"},
		{RuleID: ruleID, FeedbackType: "FALSE_POSITIVE", CodeSnippet: "testDB.QueryMock()"},
		{RuleID: ruleID, FeedbackType: "FALSE_POSITIVE", CodeSnippet: "fakeDB.RunQuery()"},
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
	}
	currentRule := &SynthesizedRuleDefinition{
		ID:              ruleID,
		NegativePattern: `(?i)dummy_test`,
	}
	reportTox := optimizer.EvaluateRuleHealth(context.Background(), ruleID, feedbacksToxic, currentRule)
	if reportTox.Grade != HealthToxic {
		t.Errorf("expected HealthToxic, got %s", reportTox.Grade)
	}
	if reportTox.AutoAction != "AUTO_DEMOTE_TO_DRAFT" {
		t.Errorf("expected AUTO_DEMOTE_TO_DRAFT, got %s", reportTox.AutoAction)
	}
	if reportTox.SynthesizedNegative == "" {
		t.Errorf("expected synthesized negative pattern to be generated")
	}

	// 3. Degraded Rule (20-35% false positive)
	feedbacksDegraded := []DeveloperRuleFeedback{
		{RuleID: ruleID, FeedbackType: "FALSE_POSITIVE", CodeSnippet: "unusualCall()"},
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
		{RuleID: ruleID, FeedbackType: "ACCEPTED"},
	}
	reportDeg := optimizer.EvaluateRuleHealth(context.Background(), ruleID, feedbacksDegraded, nil)
	if reportDeg.Grade != HealthDegraded {
		t.Errorf("expected HealthDegraded, got %s", reportDeg.Grade)
	}
	if reportDeg.AutoAction != "SYNTHESIZE_NEGATIVE_REGEX" {
		t.Errorf("expected SYNTHESIZE_NEGATIVE_REGEX, got %s", reportDeg.AutoAction)
	}
}

func TestDeepRulesEngine_ConcurrentStress(t *testing.T) {
	gen := NewDrixyRuleGenerator()
	detector := NewRuleConflictDetector()
	optimizer := NewRuleHealthOptimizer()

	var wg sync.WaitGroup
	workers := 25

	for w := 0; w < workers; w++ {
		wg.Add(3)

		// Goroutine 1: Rule Generator
		go func(workerID int) {
			defer wg.Done()
			comments := []PastReviewComment{
				{
					ID:        fmt.Sprintf("w-%d-1", workerID),
					Author:    "devA",
					FilePath:  "server/handler.go",
					Body:      "Avoid raw query here, SQL injection danger.",
					CreatedAt: time.Now(),
				},
				{
					ID:        fmt.Sprintf("w-%d-2", workerID),
					Author:    "devB",
					FilePath:  "server/store.go",
					Body:      "Raw query with concatenation is bad practice.",
					CreatedAt: time.Now(),
				},
			}
			_, _ = gen.GenerateRules(context.Background(), RuleSynthesisRequest{
				OrgID:              "stress-org",
				RepoID:             "stress-repo",
				MinClusterSize:     2,
				HistoricalComments: comments,
			})
		}(w)

		// Goroutine 2: Conflict Inspector
		go func(workerID int) {
			defer wg.Done()
			atoms := []*DrixyRuleAtom{
				{
					ID:             fmt.Sprintf("c-atom-%d", workerID),
					ParentRuleID:   uuid.New(),
					Title:          "Mutex Lock Rule",
					NormativeLevel: NormativeMust,
					Severity:       models.SeverityHigh,
					PathGlobs:      []string{"**/*.go"},
				},
				{
					ID:             fmt.Sprintf("c-atom-shadow-%d", workerID),
					ParentRuleID:   uuid.New(),
					Title:          "Mutex Lock Rule",
					NormativeLevel: NormativeMust,
					Severity:       models.SeverityLow,
					PathGlobs:      []string{"**/*.go"},
				},
			}
			_ = detector.InspectRuleCollection(atoms)
		}(w)

		// Goroutine 3: Health Optimizer
		go func(workerID int) {
			defer wg.Done()
			rID := uuid.New()
			fb := []DeveloperRuleFeedback{
				{RuleID: rID, FeedbackType: "ACCEPTED"},
				{RuleID: rID, FeedbackType: "FALSE_POSITIVE", CodeSnippet: "fakeStub()"},
			}
			_ = optimizer.EvaluateRuleHealth(context.Background(), rID, fb, nil)
		}(w)
	}

	wg.Wait()
}
