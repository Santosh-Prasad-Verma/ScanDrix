// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockOrchestratorSpecialist struct {
	name         string
	category     string
	findings     []AgentFinding
	finishReason string
	err          error
	delay        time.Duration
}

func (m *mockOrchestratorSpecialist) Name() string     { return m.name }
func (m *mockOrchestratorSpecialist) Category() string { return m.category }
func (m *mockOrchestratorSpecialist) Review(ctx context.Context, input ReviewAgentInput) (*ReviewAgentOutput, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	finish := m.finishReason
	if finish == "" {
		finish = "completed"
	}
	return &ReviewAgentOutput{
		AgentName:    m.name,
		Category:     m.category,
		Findings:     m.findings,
		FinishReason: finish,
		DurationMs:   50,
	}, nil
}

func TestGetMaxStepsForAgent(t *testing.T) {
	svc := NewReviewOrchestratorService(nil, nil, nil)

	// Fast Mode
	assert.Equal(t, 4, svc.GetMaxStepsForAgent("generalist", ReviewModeFast, 5))
	assert.Equal(t, 3, svc.GetMaxStepsForAgent("security", ReviewModeFast, 5))
	assert.Equal(t, 3, svc.GetMaxStepsForAgent("performance", ReviewModeFast, 5))

	// Deep Mode
	assert.Equal(t, 100, svc.GetMaxStepsForAgent("generalist", ReviewModeDeep, 5))
	assert.Equal(t, 100, svc.GetMaxStepsForAgent("security", ReviewModeDeep, 120))

	// Normal Mode: <= 8 files
	assert.Equal(t, 20, svc.GetMaxStepsForAgent("generalist", ReviewModeNormal, 8))
	assert.Equal(t, 12, svc.GetMaxStepsForAgent("security", ReviewModeNormal, 8))

	// Normal Mode Adaptive Scaling: 20 files (12 extra files * 0.5 = 6 extra steps)
	assert.Equal(t, 26, svc.GetMaxStepsForAgent("generalist", ReviewModeNormal, 20))
	assert.Equal(t, 18, svc.GetMaxStepsForAgent("security", ReviewModeNormal, 20))

	// Normal Mode Adaptive Capped at 100
	assert.Equal(t, 100, svc.GetMaxStepsForAgent("generalist", ReviewModeNormal, 500))
}

func TestResolveAgentsForInput(t *testing.T) {
	svc := NewReviewOrchestratorService(nil, nil, nil)

	gen := &mockOrchestratorSpecialist{name: "generalist", category: "generalist"}
	bug := &mockOrchestratorSpecialist{name: "bug", category: "bug"}
	sec := &mockOrchestratorSpecialist{name: "security", category: "security"}
	rules := &mockOrchestratorSpecialist{name: "drixy_rules", category: "drixy_rules"}

	svc.RegisterSpecialist(gen)
	svc.RegisterSpecialist(bug)
	svc.RegisterSpecialist(sec)
	svc.RegisterSpecialist(rules)

	// Normal mode with generalist registered -> selects generalist
	normalInput := ReviewAgentInput{
		ReviewOptions: ReviewOptions{
			Bug:        true,
			Security:   true,
			ReviewMode: ReviewModeNormal,
		},
	}
	active := svc.ResolveAgentsForInput(normalInput)
	require.Len(t, active, 1)
	assert.Equal(t, "generalist", active[0].Name())

	// Deep mode -> selects individual specialists
	deepInput := ReviewAgentInput{
		ReviewOptions: ReviewOptions{
			Bug:        true,
			Security:   true,
			ReviewMode: ReviewModeDeep,
		},
	}
	activeDeep := svc.ResolveAgentsForInput(deepInput)
	require.Len(t, activeDeep, 2)
	assert.Equal(t, "bug", activeDeep[0].Name())
	assert.Equal(t, "security", activeDeep[1].Name())

	// Drixy Rules appended when active rules present
	rulesInput := ReviewAgentInput{
		ReviewOptions: ReviewOptions{
			Bug:        true,
			DrixyRules: true,
			ReviewMode: ReviewModeNormal,
		},
		DrixyRules: []DrixyRule{{ID: uuid.New(), Name: "Rule 1"}},
	}
	activeRules := svc.ResolveAgentsForInput(rulesInput)
	require.Len(t, activeRules, 2)
	assert.Equal(t, "generalist", activeRules[0].Name())
	assert.Equal(t, "drixy_rules", activeRules[1].Name())
}

func TestStripFileContents(t *testing.T) {
	files := []ChangedFile{
		{
			Filename:   "handler.go",
			Content:    "package handler\nfunc Hello() {}",
			OldContent: "package handler",
			Patch:      "@@ -1 +1 @@",
			Additions:  2,
			Deletions:  1,
		},
	}

	stripped := StripFileContents(files)
	require.Len(t, stripped, 1)
	assert.Empty(t, stripped[0].Content)
	assert.Empty(t, stripped[0].OldContent)
	assert.Equal(t, "handler.go", stripped[0].Filename)
	assert.Equal(t, "@@ -1 +1 @@", stripped[0].Patch)
	assert.Equal(t, 2, stripped[0].Additions)
	assert.Equal(t, 1, stripped[0].Deletions)
}

func TestReviewOrchestratorService_Execute_DeepMode(t *testing.T) {
	svc := NewReviewOrchestratorService(nil, nil, nil)

	findingSec := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "security",
		FilePath:    "auth.go",
		StartLine:   10,
		EndLine:     12,
		Severity:    models.SeverityCritical,
		Title:       "Hardcoded Secret Key",
		Description: "Production private key exposed in source.",
		Blocking:    true,
	}

	findingBug := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "bug",
		FilePath:    "auth.go",
		StartLine:   30,
		EndLine:     32,
		Severity:    models.SeverityMedium,
		Title:       "Resource Leak",
		Description: "HTTP response body not closed.",
		Blocking:    false,
	}

	sec := &mockOrchestratorSpecialist{name: "security", category: "security", findings: []AgentFinding{findingSec}}
	bug := &mockOrchestratorSpecialist{name: "bug", category: "bug", findings: []AgentFinding{findingBug}}

	svc.RegisterSpecialist(sec)
	svc.RegisterSpecialist(bug)

	input := ReviewAgentInput{
		PRNumber: 101,
		Title:    "Update auth backend",
		ChangedFiles: []ChangedFile{
			{
				Filename: "auth.go",
				Content:  "package auth\n...",
			},
		},
		ReviewOptions: ReviewOptions{
			Bug:        true,
			Security:   true,
			ReviewMode: ReviewModeDeep,
		},
	}

	out, err := svc.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, VerdictRequestChanges, out.Verdict)
	assert.Len(t, out.Findings, 2)
	assert.Empty(t, out.Failures)
	assert.Empty(t, out.Incomplete)
	assert.Equal(t, "security", out.Findings[0].AgentName)
	assert.True(t, out.Findings[0].Blocking)
	assert.Contains(t, out.Summary, "Hardcoded Secret Key")
}

func TestReviewOrchestratorService_Execute_FailureAndIncompleteHandling(t *testing.T) {
	svc := NewReviewOrchestratorService(nil, nil, nil)

	findingBug := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "bug",
		FilePath:    "worker.go",
		StartLine:   15,
		EndLine:     18,
		Severity:    models.SeverityLow,
		Title:       "Minor logging typo",
		Description: "Misspelled log level tag.",
		Blocking:    false,
	}

	// Bug specialist was cut short by max steps
	bug := &mockOrchestratorSpecialist{
		name:         "bug",
		category:     "bug",
		findings:     []AgentFinding{findingBug},
		finishReason: "max-steps",
	}

	// Security specialist failed with an unrecoverable error
	sec := &mockOrchestratorSpecialist{
		name:     "security",
		category: "security",
		err:      errors.New("API rate limit exceeded"),
	}

	svc.RegisterSpecialist(bug)
	svc.RegisterSpecialist(sec)

	input := ReviewAgentInput{
		PRNumber: 102,
		Title:    "Worker refactoring",
		ChangedFiles: []ChangedFile{
			{
				Filename: "worker.go",
				Content:  "package worker\n...",
			},
		},
		ReviewOptions: ReviewOptions{
			Bug:        true,
			Security:   true,
			ReviewMode: ReviewModeDeep,
		},
	}

	out, err := svc.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, out)

	// Findings from incomplete agent are preserved
	assert.Len(t, out.Findings, 1)
	assert.Equal(t, "worker.go", out.Findings[0].FilePath)

	// Telemetry accurately tracks 1 failure and 1 incomplete
	require.Len(t, out.Failures, 1)
	assert.Equal(t, "security", out.Failures[0].AgentName)
	assert.Contains(t, out.Failures[0].Error, "rate limit exceeded")

	require.Len(t, out.Incomplete, 1)
	assert.Equal(t, "bug", out.Incomplete[0].AgentName)
	assert.Equal(t, "max-steps", out.Incomplete[0].FinishReason)
	assert.Equal(t, 1, out.Incomplete[0].SuggestionsFound)
}

func TestReviewOrchestratorService_Execute_NoAgentsEnabled(t *testing.T) {
	svc := NewReviewOrchestratorService(nil, nil, nil)

	input := ReviewAgentInput{
		PRNumber: 103,
		Title:    "Docs only update",
		ChangedFiles: []ChangedFile{
			{Filename: "README.md", Content: "# Readme"},
		},
		ReviewOptions: ReviewOptions{
			Bug:         false,
			Security:    false,
			Performance: false,
			ReviewMode:  ReviewModeFast,
		},
	}

	out, err := svc.Execute(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, VerdictApprove, out.Verdict)
	assert.Empty(t, out.Findings)
	assert.Contains(t, out.Summary, "No review categories enabled")
}
