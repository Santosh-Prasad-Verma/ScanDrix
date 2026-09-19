// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package specialists

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) GenerateResponse(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestSecuritySpecialistAgent(t *testing.T) {
	mockResp := `{
		"findings": [
			{
				"file_path": "internal/auth/handler.go",
				"start_line": 45,
				"end_line": 50,
				"severity": "CRITICAL",
				"title": "SQL Injection in User Lookup",
				"description": "Unescaped username parameter used in query.",
				"blocking": true
			}
		]
	}`

	agent := NewSecuritySpecialistAgent(&mockLLMClient{response: mockResp})
	assert.Equal(t, "security", agent.Name())
	assert.Equal(t, "security", agent.Category())

	input := orchestrator.ReviewAgentInput{
		PRNumber: 101,
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "internal/auth/handler.go", Patch: "+ query := fmt.Sprintf(\"SELECT * FROM users WHERE name='%s'\", name)"},
		},
	}

	out, err := agent.Review(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)

	assert.Equal(t, "security", out.Findings[0].AgentName)
	assert.Equal(t, models.SeverityCritical, out.Findings[0].Severity)
	assert.True(t, out.Findings[0].Blocking)
}

func TestPerformanceSpecialistAgent(t *testing.T) {
	mockResp := `{
		"findings": [
			{
				"file_path": "internal/orders/service.go",
				"start_line": 80,
				"end_line": 85,
				"severity": "HIGH",
				"title": "N+1 Queries in Loop",
				"description": "Fetching customer profile in a loop."
			}
		]
	}`

	agent := NewPerformanceSpecialistAgent(&mockLLMClient{response: mockResp})
	assert.Equal(t, "performance", agent.Name())

	input := orchestrator.ReviewAgentInput{
		PRNumber: 102,
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "internal/orders/service.go", Patch: "+ for _, o := range orders { repo.GetCustomer(o.ID) }"},
		},
	}

	out, err := agent.Review(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "performance", out.Findings[0].AgentName)
	assert.Equal(t, models.SeverityHigh, out.Findings[0].Severity)
}

func TestBugSpecialistAgent(t *testing.T) {
	mockResp := `{
		"findings": [
			{
				"file_path": "internal/cache/cache.go",
				"start_line": 20,
				"end_line": 25,
				"severity": "CRITICAL",
				"title": "Data Race on Map Access",
				"description": "Map accessed concurrently without mutex."
			}
		]
	}`

	agent := NewBugSpecialistAgent(&mockLLMClient{response: mockResp})
	assert.Equal(t, "bug", agent.Name())

	out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
		PRNumber: 103,
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "internal/cache/cache.go", Patch: "+ c.items[key] = val"},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "bug", out.Findings[0].AgentName)
	assert.Equal(t, models.SeverityCritical, out.Findings[0].Severity)
}

func TestArchitectureSpecialistAgent(t *testing.T) {
	mockResp := `{
		"findings": [
			{
				"file_path": "internal/domain/user.go",
				"start_line": 5,
				"end_line": 8,
				"severity": "MEDIUM",
				"title": "Clean Architecture Violation",
				"description": "Domain imports infrastructure sql package."
			}
		]
	}`

	agent := NewArchitectureSpecialistAgent(&mockLLMClient{response: mockResp})
	assert.Equal(t, "architecture", agent.Name())

	out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
		PRNumber: 104,
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "internal/domain/user.go", Patch: "+ import \"database/sql\""},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "architecture", out.Findings[0].AgentName)
}

func TestBusinessLogicSpecialistAgent(t *testing.T) {
	agent := NewBusinessLogicSpecialistAgent(nil)

	// Test ticket key extraction
	desc := "Fixes SCANDRIX-1024 and relates to PROJ_A-42 and #450"
	keys := agent.ExtractTicketKeys(desc)
	assert.Contains(t, keys, "SCANDRIX-1024")
	assert.Contains(t, keys, "PROJ_A-42")
	assert.Contains(t, keys, "#450")

	// Test requirements detection
	assert.True(t, agent.HasRequirementsDeclared("Acceptance Criteria: All users must verify email"))
	assert.False(t, agent.HasRequirementsDeclared("Just a regular fix"))

	// Test review with no requirements/tickets (skips gracefully)
	out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
		PRNumber:    105,
		Title:       "minor docs change",
		Description: "fix typo",
	})
	require.NoError(t, err)
	assert.Len(t, out.Findings, 0)
}

func TestDrixyRulesSpecialistAgent(t *testing.T) {
	ruleID := uuid.New()
	rule := orchestrator.DrixyRule{
		ID:        ruleID,
		Name:      "Require Context in Handlers",
		Severity:  models.SeverityHigh,
		Scope:     "file",
		PathGlobs: []string{"*.go"},
		IsActive:  true,
	}

	mockExec := &mockJudgeExecutor{
		response: `{
			"violations": [
				{
					"rule_id": 1,
					"relevant_lines_start": 10,
					"relevant_lines_end": 12,
					"suggestion_content": "Handler does not accept context.Context"
				}
			]
		}`,
	}

	agent := NewDrixyRulesSpecialistAgent(mockExec)
	assert.Equal(t, "drixy_rules", agent.Name())

	out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
		PRNumber: 106,
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "handler.go", Patch: "+ func Handle() {}"},
		},
		DrixyRules: []orchestrator.DrixyRule{rule},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "drixy_rules", out.Findings[0].AgentName)
	assert.Equal(t, &ruleID, out.Findings[0].RuleID)
}

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

func TestGeneralistSpecialistAgent(t *testing.T) {
	mockResp := `{
		"findings": [
			{
				"file_path": "services/order.go",
				"start_line": 25,
				"end_line": 30,
				"severity": "HIGH",
				"category": "bug",
				"title": "Missing Transaction Rollback",
				"description": "Error returns without aborting the active database transaction.",
				"blocking": true
			}
		]
	}`

	agent := NewGeneralistSpecialistAgent(&mockLLMClient{response: mockResp})
	assert.Equal(t, "generalist", agent.Name())
	assert.Equal(t, "generalist", agent.Category())

	out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
		PRNumber: 107,
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "services/order.go", Patch: "+ if err != nil { return err }"},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "generalist", out.Findings[0].AgentName)
	assert.Equal(t, models.SeverityHigh, out.Findings[0].Severity)
	assert.True(t, out.Findings[0].Blocking)
}

