// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package contextpack

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferenceDetector_DetectReferences(t *testing.T) {
	detector := NewReferenceDetector()

	title := "fix(auth): resolve session expiration [SCANDRIX-1042] (#45)"
	desc := `Closes #46 and links to JIRA PROJ-99.
See architecture decisions in docs/auth_strategy.md.
Also ref commit 7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b.
Ignore UTF-8 and SHA-256 encoding notes.`

	refs := detector.DetectReferences(title, desc)

	// Tickets
	assert.Contains(t, refs.TicketKeys, "SCANDRIX-1042")
	assert.Contains(t, refs.TicketKeys, "PROJ-99")
	assert.NotContains(t, refs.TicketKeys, "UTF-8")
	assert.NotContains(t, refs.TicketKeys, "SHA-256")

	// Issues
	assert.Contains(t, refs.IssueNumbers, 45)
	assert.Contains(t, refs.IssueNumbers, 46)

	// Doc paths
	assert.Contains(t, refs.DocPaths, "docs/auth_strategy.md")

	// SHAs
	assert.Contains(t, refs.CommitSHAs, "7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b")
}

func TestBudgetAllocator_LayerCapAndPriorityDrop(t *testing.T) {
	allocator := NewBudgetAllocator()

	config := ContextBudgetConfig{
		TotalBudgetTokens: 1000,
		LayerBudgets: map[ContextLayerKind]int{
			LayerDiff:        600,
			LayerConventions: 200,
		},
	}

	layers := []ContextLayer{
		{
			Kind:     LayerDiff,
			Name:     "Diff",
			Content:  string(make([]byte, 2800)), // ~1000 tokens code, exceeds 600 cap
			Priority: 100,
		},
		{
			Kind:     LayerTraceDecision,
			Name:     "Trace",
			Content:  string(make([]byte, 1140)), // ~300 tokens prose
			Priority: 90,
		},
		{
			Kind:     LayerConventions,
			Name:     "Conventions",
			Content:  string(make([]byte, 1900)), // ~500 tokens prose, exceeds 200 cap
			Priority: 50,
		},
	}

	allocated, totalTokens := allocator.Allocate(layers, config)

	// Conventions layer should have been dropped to satisfy total budget 1000 (600 + 300 = 900 <= 1000)
	var conventionsLayer *ContextLayer
	for i := range allocated {
		if allocated[i].Kind == LayerConventions {
			conventionsLayer = &allocated[i]
		}
	}
	require.NotNil(t, conventionsLayer)
	assert.True(t, conventionsLayer.DroppedForBudget)

	assert.LessOrEqual(t, totalTokens, 1000)
}

func TestTraceDecisionStore_IndexingAndRetrieval(t *testing.T) {
	store := NewTraceDecisionStore()
	orgID := "org-1"
	repoID := "repo-1"

	decision := TraceDecision{
		ID:          uuid.New(),
		DecisionKey: "ADR-005",
		Title:       "Use Argon2id for Password Hashing",
		Summary:     "Argon2id protects against GPU brute force.",
		Rationale:   "OWASP guidelines require memory-hard hashing.",
		Files:       []string{"internal/auth/hasher.go", "pkg/security/pass.go"},
	}

	store.AddDecision(orgID, repoID, decision)

	// Query with matching file
	matched := store.GetDecisionsForFiles(orgID, repoID, []string{"internal/auth/hasher.go"})
	require.Len(t, matched, 1)
	assert.Equal(t, "ADR-005", matched[0].DecisionKey)

	// Query with relative path leading slash
	matchedSlash := store.GetDecisionsForFiles(orgID, repoID, []string{"/pkg/security/pass.go"})
	require.Len(t, matchedSlash, 1)
	assert.Equal(t, "ADR-005", matchedSlash[0].DecisionKey)

	// Query with unmatching file
	unmatched := store.GetDecisionsForFiles(orgID, repoID, []string{"cmd/main.go"})
	assert.Empty(t, unmatched)

	// Test pipeline interface
	info, err := store.LoadDecisionsForFiles(context.Background(), orgID, repoID, []string{"internal/auth/hasher.go"})
	require.NoError(t, err)
	require.Len(t, info, 1)
	assert.Equal(t, "ADR-005", info[0].DecisionKey)
}

func TestContextPackAssembler_Assemble(t *testing.T) {
	assembler := NewContextPackAssembler(nil)

	input := AssemblerInput{
		DiffContent: "diff --git a/main.go b/main.go\n+func Run() {}",
		Tickets: []TicketContext{
			{
				IssueKey:           "SCANDRIX-10",
				Provider:           "jira",
				Title:              "Add Healthcheck Endpoint",
				Description:        "Provide /healthz for Kubernetes liveness probes.",
				AcceptanceCriteria: []string{"Returns HTTP 200 with OK", "Responds within 10ms"},
			},
		},
		TraceDecisions: []TraceDecision{
			{
				DecisionKey: "ADR-012",
				Title:       "Healthcheck isolation",
				Summary:     "Liveness probe must not depend on database connection.",
				Rationale:   "Prevent cascade restarts during DB maintenance.",
				Files:       []string{"main.go"},
			},
		},
		KnowledgeGraphPrompt: "=== CALL CONTEXT: Run() called by cmd/server.go ===",
		RulesSummary:         "Rules: 1 active custom rule for health check",
	}

	config := DefaultContextBudgetConfig()
	pack, err := assembler.Assemble(context.Background(), input, config)
	require.NoError(t, err)
	require.NotNil(t, pack)

	assert.NotEmpty(t, pack.AssembledPrompt)
	assert.Contains(t, pack.AssembledPrompt, "CONTEXT LAYER: UNIFIED DIFF")
	assert.Contains(t, pack.AssembledPrompt, "CONTEXT LAYER: TRACE ARCHITECTURAL DECISIONS")
	assert.Contains(t, pack.AssembledPrompt, "ADR-012")
	assert.Contains(t, pack.AssembledPrompt, "SCANDRIX-10")
	assert.Contains(t, pack.AssembledPrompt, "Liveness probe must not depend on database connection")
	assert.Contains(t, pack.AssembledPrompt, "KNOWLEDGE GRAPH")
	assert.Equal(t, 0, pack.DroppedLayersCount)
}
