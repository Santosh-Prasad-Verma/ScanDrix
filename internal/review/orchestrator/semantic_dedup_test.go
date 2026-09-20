// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSemanticDeduplicator_DeduplicateFindings(t *testing.T) {
	dedup := NewSemanticDeduplicator(nil)

	// Two findings reporting the same nil pointer dereference on lines 15-18
	f1 := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "bug",
		FilePath:    "internal/service.go",
		StartLine:   15,
		EndLine:     18,
		Severity:    models.SeverityHigh,
		Title:       "Potential nil pointer dereference on user pointer",
		Description: "The user variable is dereferenced before checking if it is nil.",
		Remediation: "if user == nil { return nil }",
		Blocking:    true,
	}

	f2 := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "security",
		FilePath:    "internal/service.go",
		StartLine:   16,
		EndLine:     19,
		Severity:    models.SeverityCritical, // Security upgraded severity
		Title:       "Unchecked pointer dereference in user handler",
		Description: "Dereferencing user pointer without validation can cause panic DoS.",
		Remediation: "if user == nil { return errors.New(\"unauthorized\") }",
		Blocking:    true,
	}

	// Completely unrelated finding in another file
	f3 := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "performance",
		FilePath:    "internal/query.go",
		StartLine:   50,
		EndLine:     55,
		Severity:    models.SeverityMedium,
		Title:       "N+1 SQL query inside loop",
		Description: "LoadUsers is called on each iteration of the order items loop.",
	}

	findings := []AgentFinding{f1, f2, f3}
	deduped, dropped := dedup.DeduplicateFindings(findings)

	require.Len(t, deduped, 2)
	assert.Equal(t, 1, dropped)

	// Check that the merged finding preserved Critical severity from security agent
	var mergedFinding AgentFinding
	for _, f := range deduped {
		if f.FilePath == "internal/service.go" {
			mergedFinding = f
		}
	}
	assert.Equal(t, models.SeverityCritical, mergedFinding.Severity)
	assert.True(t, mergedFinding.Blocking)
	assert.Equal(t, 15, mergedFinding.StartLine)
	assert.Equal(t, 19, mergedFinding.EndLine)
	assert.Contains(t, mergedFinding.ContributingAgents, "bug")
	assert.Contains(t, mergedFinding.ContributingAgents, "security")
}
