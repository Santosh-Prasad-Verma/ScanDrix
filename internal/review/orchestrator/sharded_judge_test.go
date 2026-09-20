// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockJudgeExecutor struct {
	response string
	err      error
}

func (m *mockJudgeExecutor) ExecuteShard(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestDeterministicShardedJudge_BuildShards(t *testing.T) {
	rule1 := DrixyRule{
		ID:        uuid.New(),
		Name:      "No Raw SQL",
		Severity:  models.SeverityCritical,
		Scope:     "file",
		PathGlobs: []string{"internal/database/*", "*.sql"},
		IsActive:  true,
	}
	rule2 := DrixyRule{
		ID:        uuid.New(),
		Name:      "Use Chi Router Context",
		Severity:  models.SeverityMedium,
		Scope:     "file",
		PathGlobs: []string{"internal/api/*"},
		IsActive:  true,
	}
	prRule := DrixyRule{
		ID:       uuid.New(),
		Name:     "Require PR Ticket Link",
		Severity: models.SeverityHigh,
		Scope:    "pull_request",
		IsActive: true,
	}

	files := []ChangedFile{
		{Filename: "internal/database/user_repo.go"},
		{Filename: "internal/api/handler.go"},
		{Filename: "README.md"},
	}

	judge := NewDeterministicShardedJudge(nil)
	shards, prRules := judge.BuildShards(files, []DrixyRule{rule1, rule2, prRule})

	require.Len(t, shards, 2)
	assert.Equal(t, "internal/database/user_repo.go", shards[0].File.Filename)
	assert.Equal(t, "No Raw SQL", shards[0].Rules[0].Name)

	assert.Equal(t, "internal/api/handler.go", shards[1].File.Filename)
	assert.Equal(t, "Use Chi Router Context", shards[1].Rules[0].Name)

	require.Len(t, prRules, 1)
	assert.Equal(t, "Require PR Ticket Link", prRules[0].Name)
}

func TestDeterministicShardedJudge_EvaluateAll(t *testing.T) {
	ruleID := uuid.New()
	rule := DrixyRule{
		ID:        ruleID,
		Name:      "No Hardcoded Secrets",
		Severity:  models.SeverityCritical,
		Scope:     "file",
		PathGlobs: []string{"*.go"},
		IsActive:  true,
	}

	mockResp := `{
		"violations": [
			{
				"rule_id": 1,
				"relevant_lines_start": 42,
				"relevant_lines_end": 45,
				"language": "go",
				"suggestion_content": "Hardcoded secret key found in source file.",
				"improved_code": "secret := os.Getenv(\"API_KEY\")"
			}
		]
	}`

	mockExec := &mockJudgeExecutor{response: mockResp}
	judge := NewDeterministicShardedJudge(mockExec)

	input := ReviewAgentInput{
		ChangedFiles: []ChangedFile{
			{
				Filename: "internal/auth/token.go",
				Content:  `const apiKey = "sk_live_12345"`,
			},
		},
		DrixyRules: []DrixyRule{rule},
	}

	findings, err := judge.EvaluateAll(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, findings, 1)

	f := findings[0]
	assert.Equal(t, "internal/auth/token.go", f.FilePath)
	assert.Equal(t, 42, f.StartLine)
	assert.Equal(t, 45, f.EndLine)
	assert.Equal(t, models.SeverityCritical, f.Severity)
	assert.Equal(t, "drixy_rules", f.AgentName)
	assert.Equal(t, &ruleID, f.RuleID)
	assert.True(t, f.Blocking)
}

func TestRuleAppliesToFile_CommaSeparated(t *testing.T) {
	pattern := "src/**/*.go, pkg/*.go, internal/auth/*"

	assert.True(t, RuleAppliesToFile("src/api/v1/handler.go", pattern))
	assert.True(t, RuleAppliesToFile("pkg/util.go", pattern))
	assert.True(t, RuleAppliesToFile("internal/auth/token.go", pattern))
	assert.False(t, RuleAppliesToFile("frontend/app.tsx", pattern))
	assert.False(t, RuleAppliesToFile("internal/billing/stripe.go", pattern))
}

func TestInlineRuleReferences(t *testing.T) {
	rules := []DrixyRule{
		{
			ID:         uuid.New(),
			Name:       "Follow Auth Convention",
			Prompt:     "Always hash passwords with Argon2id.",
			SourcePath: "docs/auth_architecture.md",
		},
		{
			ID:     uuid.New(),
			Name:   "No Source Path",
			Prompt: "Do not use MD5.",
		},
	}

	reader := func(path string, start, end int) (string, error) {
		if path == "docs/auth_architecture.md" {
			return "Auth standard requires argon2.IDKey with m=65536, t=3, p=4.", nil
		}
		return "", fmt.Errorf("not found")
	}

	inlined := InlineRuleReferences(rules, reader, 6000)
	require.Len(t, inlined, 2)
	assert.Contains(t, inlined[0].Prompt, "Authoritative convention referenced by this rule — from `docs/auth_architecture.md`")
	assert.Contains(t, inlined[0].Prompt, "argon2.IDKey")
	assert.NotContains(t, inlined[1].Prompt, "Authoritative convention")
}

func TestInlineLoadedReferences(t *testing.T) {
	ruleID := uuid.New()
	rules := []DrixyRule{
		{
			ID:     ruleID,
			Name:   "Logging Standard",
			Prompt: "Use structured zap logging.",
		},
	}

	refMap := map[string][]LoadedRuleReference{
		ruleID.String(): {
			{
				FilePath: "pkg/log/logger.go",
				Content:  "func NewProductionLogger() *zap.Logger { ... }",
			},
		},
	}

	inlined := InlineLoadedReferences(rules, refMap, 6000)
	require.Len(t, inlined, 1)
	assert.Contains(t, inlined[0].Prompt, "Authoritative convention referenced by this rule — from `pkg/log/logger.go`")
	assert.Contains(t, inlined[0].Prompt, "NewProductionLogger")
}

func TestBuildPRShardUserPrompt_DiffBudgetCapping(t *testing.T) {
	// Create files whose diffs exceed PRShardDiffBudgetChars (150,000)
	largeDiff1 := strings.Repeat("line in diff\n", 8000) // ~104,000 chars
	largeDiff2 := strings.Repeat("line in diff\n", 8000) // ~104,000 chars (exceeds budget)

	files := []ChangedFile{
		{Filename: "file1.go", Patch: largeDiff1},
		{Filename: "file2.go", Patch: largeDiff2},
	}

	rules := []DrixyRule{
		{ID: uuid.New(), Name: "PR Architecture Rule", Scope: "pull_request"},
	}

	prompt := BuildPRShardUserPrompt(files, rules, "Add feature", "PR description", "Portuguese (Brazil)")

	assert.Contains(t, prompt, "diff omitted — PR diff budget exceeded")
	assert.Contains(t, prompt, "Respond in Portuguese (Brazil)")
}

