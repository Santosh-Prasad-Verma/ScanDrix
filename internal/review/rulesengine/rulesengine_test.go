// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/rulesengine"
	"github.com/scandrix/backend/pkg/models"
)

func TestDetectorEvaluator_MatchesAdditionsAndSuppressesNegatives(t *testing.T) {
	evaluator := rulesengine.NewDetectorEvaluator()
	reviewID := uuid.New()
	workspaceID := uuid.New()

	detectorRule := &rulesengine.DrixyRule{
		ID:          uuid.New(),
		Slug:        "no-raw-sql-concat",
		Title:       "Prevent SQL Injection",
		Description: "Raw query string concat is dangerous",
		Severity:    models.SeverityCritical,
		Scope:       rulesengine.ScopeFile,
		PathGlobs:   []string{"**/*.go"},
		Status:      rulesengine.StatusActive,
		Detector: &rulesengine.CompiledRuleDetector{
			Type:            rulesengine.DetectorRegex,
			Pattern:         `db\.Query\(fmt\.Sprintf\(`,
			NegativePattern: `test_mock_db`,
			Reason:          "Unescaped SQL query string formatting",
		},
		RemediationHint: "Use parameterized query placeholders",
	}

	patches := []*diff.FilePatch{
		{
			OldPath: "services/user.go",
			NewPath: "services/user.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "func GetUser(id string) {"},
						// Deleted line with violation - must NOT trigger finding!
						{Type: diff.LineDeletion, Content: "\trows, _ := db.Query(fmt.Sprintf(\"SELECT * FROM users WHERE id = %s\", id))"},
						// Added line with violation - MUST trigger finding!
						{Type: diff.LineAddition, NewLineNo: 42, Content: "\trows, _ := db.Query(fmt.Sprintf(\"SELECT * FROM accounts WHERE id = %s\", id))"},
						// Added line matching negative suppression - must NOT trigger!
						{Type: diff.LineAddition, NewLineNo: 43, Content: "\trows, _ := test_mock_db.Query(fmt.Sprintf(\"SELECT 1\"))"},
					},
				},
			},
		},
		{
			OldPath: "frontend/app.ts",
			NewPath: "frontend/app.ts",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						// Path doesn't match **/*.go - must NOT trigger!
						{Type: diff.LineAddition, NewLineNo: 10, Content: "db.Query(fmt.Sprintf("},
					},
				},
			},
		},
	}

	findings := evaluator.EvaluateRules(context.Background(), reviewID, workspaceID, []*rulesengine.DrixyRule{detectorRule}, patches)
	require.Len(t, findings, 1)

	f := findings[0]
	assert.Equal(t, "services/user.go", f.FilePath)
	assert.Equal(t, 42, f.StartLine)
	assert.Equal(t, 42, f.EndLine)
	assert.Equal(t, models.SeverityCritical, f.Severity)
	assert.Equal(t, "Prevent SQL Injection", f.Title)
	assert.Contains(t, f.Description, "Unescaped SQL query string formatting")
	assert.Contains(t, f.Remediation, "parameterized query")
	assert.NotEmpty(t, f.Fingerprint)
}

func TestRulesCatalog_BuiltinsAndScoping(t *testing.T) {
	catalog := rulesengine.NewRulesCatalog()

	// Verify builtins
	libraryRules := catalog.GetLibraryRules()
	assert.GreaterOrEqual(t, len(libraryRules), 4)

	orgID := uuid.New()
	teamID := uuid.New()
	repoID := "payments-service"

	// Add Org Rule
	orgRule := &rulesengine.DrixyRule{
		OrgID:       orgID,
		Slug:        "org-rule-1",
		Title:       "Org Level Rule",
		Severity:    models.SeverityHigh,
		Scope:       rulesengine.ScopeFile,
		Status:      rulesengine.StatusActive,
		Inheritable: true,
	}
	catalog.AddRule(orgRule)

	// Add Team Rule
	teamRule := &rulesengine.DrixyRule{
		OrgID:    orgID,
		TeamID:   &teamID,
		Slug:     "team-rule-1",
		Title:    "Team Level Rule",
		Severity: models.SeverityMedium,
		Scope:    rulesengine.ScopeFile,
		Status:   rulesengine.StatusActive,
	}
	catalog.AddRule(teamRule)

	// Add Repo Rule
	repoRule := &rulesengine.DrixyRule{
		OrgID:    orgID,
		RepoID:   repoID,
		Slug:     "repo-rule-1",
		Title:    "Repo Level Rule",
		Severity: models.SeverityLow,
		Scope:    rulesengine.ScopeFile,
		Status:   rulesengine.StatusActive,
	}
	catalog.AddRule(repoRule)

	assert.Len(t, catalog.GetRulesForOrg(orgID), 1)
	assert.Len(t, catalog.GetRulesForTeam(orgID, teamID), 1)
	assert.Len(t, catalog.GetRulesForRepo(orgID, repoID), 1)

	found, ok := catalog.GetRuleByID(orgRule.ID)
	require.True(t, ok)
	assert.Equal(t, "org-rule-1", found.Slug)
}

func TestInheritanceResolver_CascadingAndPrecedence(t *testing.T) {
	catalog := rulesengine.NewRulesCatalog()
	resolver := rulesengine.NewInheritanceResolver(catalog)

	orgID := uuid.New()
	teamID := uuid.New()
	repoID := "order-service"

	// 1. Org Rule with slug "logging-policy"
	catalog.AddRule(&rulesengine.DrixyRule{
		OrgID:       orgID,
		Slug:        "logging-policy",
		Title:       "Standard Org Logging",
		Description: "Must use JSON logger",
		Severity:    models.SeverityMedium,
		Status:      rulesengine.StatusActive,
		Inheritable: true,
	})

	// 2. Repo Rule with same slug "logging-policy" - overrides Org rule!
	catalog.AddRule(&rulesengine.DrixyRule{
		OrgID:       orgID,
		RepoID:      repoID,
		Slug:        "logging-policy",
		Title:       "High Performance Logger for Order Service",
		Description: "Must use zero-allocation logger",
		Severity:    models.SeverityCritical,
		Status:      rulesengine.StatusActive,
	})

	// 3. Org Rule excluded by this repo
	catalog.AddRule(&rulesengine.DrixyRule{
		OrgID:         orgID,
		Slug:          "strict-docstrings",
		Title:         "Enforce Docstrings",
		Status:        rulesengine.StatusActive,
		Inheritable:   true,
		ExcludedRepos: []string{repoID},
	})

	// 4. In-Repo Rule with mechanical detector
	inRepoRule := &rulesengine.DrixyRule{
		Slug:        "no-panic-calls",
		Title:       "No Panic in Production",
		Description: "Never call panic()",
		Severity:    models.SeverityCritical,
		Status:      rulesengine.StatusActive,
		PathGlobs:   []string{"**/*.go"},
		Detector: &rulesengine.CompiledRuleDetector{
			Type:    rulesengine.DetectorRegex,
			Pattern: `\bpanic\(`,
		},
	}

	result := resolver.ResolveActiveRules(context.Background(), rulesengine.InheritanceInput{
		WorkspaceID:  orgID,
		TeamID:       &teamID,
		RepositoryID: repoID,
		ChangedFiles: []string{"services/orders.go"},
		InRepoRules:  []*rulesengine.DrixyRule{inRepoRule},
	})

	require.NotNil(t, result)
	assert.GreaterOrEqual(t, result.TotalActive, 2)
	assert.Greater(t, result.OverriddenCount, 0)

	// Verify repo override won
	var loggingRule *rulesengine.DrixyRule
	for _, r := range result.SemanticRules {
		if r.Slug == "logging-policy" {
			loggingRule = r
			break
		}
	}
	require.NotNil(t, loggingRule)
	assert.Equal(t, "High Performance Logger for Order Service", loggingRule.Title)
	assert.Equal(t, models.SeverityCritical, loggingRule.Severity)

	// Verify excluded rule was omitted
	for _, r := range result.SemanticRules {
		assert.NotEqual(t, "strict-docstrings", r.Slug)
	}

	// Verify in-repo mechanical rule was partitioned into MechanicalRules
	var foundMechanical *rulesengine.DrixyRule
	for _, r := range result.MechanicalRules {
		if r.Slug == "no-panic-calls" {
			foundMechanical = r
			break
		}
	}
	require.NotNil(t, foundMechanical)
	assert.Equal(t, "no-panic-calls", foundMechanical.Slug)
}

func TestInRepoRulesScanner_MarkdownAndJSON(t *testing.T) {
	scanner := rulesengine.NewInRepoRulesScanner()

	mdContent := `---
title: "Enforce Error Wrapping"
slug: "enforce-error-wrapping"
severity: "high"
scope: "file"
paths:
  - "**/*.go"
detector:
  type: "regex"
  pattern: "return err$"
  reason: "Unwrapped naked error returned"
remediation_hint: "Use fmt.Errorf(\"action failed: %w\", err)"
---
All Go functions returning errors must wrap them with context using %w.
`

	rule, err := scanner.ParseRuleFile(".scandrix/rules/errors.md", []byte(mdContent))
	require.NoError(t, err)
	require.NotNil(t, rule)

	assert.Equal(t, "enforce-error-wrapping", rule.Slug)
	assert.Equal(t, "Enforce Error Wrapping", rule.Title)
	assert.Equal(t, models.SeverityHigh, rule.Severity)
	assert.Contains(t, rule.Description, "All Go functions returning errors")
	assert.Contains(t, rule.PathGlobs, "**/*.go")
	require.NotNil(t, rule.Detector)
	assert.Equal(t, "return err$", rule.Detector.Pattern)

	jsonContent := `{
		"title": "Forbidden Math Random",
		"slug": "no-math-random",
		"severity": "critical",
		"paths": ["**/*.ts", "**/*.js"],
		"detector": {
			"pattern": "Math\\.random\\(\\)"
		},
		"description": "Math.random is not cryptographically secure"
	}`

	jsonRule, err := scanner.ParseRuleFile(".drixy/rules/crypto.json", []byte(jsonContent))
	require.NoError(t, err)
	require.NotNil(t, jsonRule)

	assert.Equal(t, "no-math-random", jsonRule.Slug)
	assert.Equal(t, models.SeverityCritical, jsonRule.Severity)
	assert.Contains(t, jsonRule.Description, "cryptographically secure")
	require.NotNil(t, jsonRule.Detector)
}

func TestInRepoRulesScanner_ScanPatches(t *testing.T) {
	scanner := rulesengine.NewInRepoRulesScanner()

	patches := []*diff.FilePatch{
		{
			OldPath: ".scandrix/rules/custom_audit.md",
			NewPath: ".scandrix/rules/custom_audit.md",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAddition, Content: "---"},
						{Type: diff.LineAddition, Content: "title: Audit Rule"},
						{Type: diff.LineAddition, Content: "severity: low"},
						{Type: diff.LineAddition, Content: "---"},
						{Type: diff.LineAddition, Content: "Audit every commit"},
					},
				},
			},
		},
	}

	rules, err := scanner.ScanPatches(patches)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "Audit Rule", rules[0].Title)
	assert.Equal(t, models.SeverityLow, rules[0].Severity)
}
