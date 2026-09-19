// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSpecialist struct {
	name     string
	category string
	output   *ReviewAgentOutput
	err      error
}

func (m *mockSpecialist) Name() string     { return m.name }
func (m *mockSpecialist) Category() string { return m.category }
func (m *mockSpecialist) Review(ctx context.Context, input ReviewAgentInput) (*ReviewAgentOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.output, nil
}

func TestMultiAgentCoordinator_CoordinateReview(t *testing.T) {
	coord := NewMultiAgentCoordinator(nil, nil, nil)

	secFinding := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "security",
		FilePath:    "main.go",
		StartLine:   10,
		EndLine:     15,
		Severity:    models.SeverityCritical,
		Title:       "Critical vulnerability",
		Description: "Buffer overflow risk",
		Blocking:    true,
	}

	secSpec := &mockSpecialist{
		name:     "security",
		category: "security",
		output: &ReviewAgentOutput{
			AgentName: "security",
			Findings:  []AgentFinding{secFinding},
		},
	}

	bugFinding := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "bug",
		FilePath:    "main.go",
		StartLine:   25,
		EndLine:     30,
		Severity:    models.SeverityMedium,
		Title:       "Potential nil pointer",
		Description: "Variable may be nil",
		Blocking:    false,
	}

	bugSpec := &mockSpecialist{
		name:     "bug",
		category: "bug",
		output: &ReviewAgentOutput{
			AgentName: "bug",
			Findings:  []AgentFinding{bugFinding},
		},
	}

	coord.RegisterSpecialist(secSpec)
	coord.RegisterSpecialist(bugSpec)

	input := ReviewAgentInput{
		PRNumber: 1,
		Title:    "Initial setup",
		ChangedFiles: []ChangedFile{
			{
				Filename: "main.go",
				Content:  "package main\n...",
			},
		},
		ReviewOptions: DefaultReviewOptions(),
	}

	res, err := coord.CoordinateReview(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, VerdictRequestChanges, res.Verdict)
	assert.Len(t, res.Findings, 2)
	assert.Len(t, res.AgentOutputs, 2)
	assert.Len(t, res.Failures, 0)
	assert.True(t, res.TotalDurationMs >= 0)
	assert.Contains(t, res.Summary, "ScanDrix AI Review: CHANGES REQUESTED")
}
