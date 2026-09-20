// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package trace_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/trace"
	"github.com/scandrix/backend/pkg/models"
)

func TestTraceStore_SaveAndQueryDecisions(t *testing.T) {
	ctx := context.Background()
	store := trace.NewTraceStore()
	orgID := "org-alpha"
	repoID := "repo-core"

	// 1. Scoped to pkg/auth/** (pinned)
	d1 := &trace.TraceDecision{
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-0001",
		Title:       "Use Argon2id for Password Hashing",
		Decision:    "Always hash passwords with argon2id, never bcrypt or SHA.",
		Type:        trace.DecisionSecurityPolicy,
		Status:      trace.StatusAccepted,
		Scope:       []string{"pkg/auth/**"},
		Confidence:  0.95,
		Pinned:      true,
	}

	// 2. Scoped to services/billing/** (unpinned, high confidence)
	d2 := &trace.TraceDecision{
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-0002",
		Title:       "Idempotent Payment Webhooks",
		Decision:    "All payment webhook handlers must acquire an outbox mutex lock.",
		Type:        trace.DecisionArchitectural,
		Status:      trace.StatusAccepted,
		Scope:       []string{"services/billing/**"},
		Confidence:  0.90,
		Pinned:      false,
	}

	// 3. Global repository decision (low confidence, unpinned)
	d3 := &trace.TraceDecision{
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-0003",
		Title:       "Bounded Context Timeouts",
		Decision:    "Keep the timeout bounded on all external network calls.",
		Type:        trace.DecisionConvention,
		Status:      trace.StatusAccepted,
		Confidence:  0.60,
		Pinned:      false,
	}

	require.NoError(t, store.SaveDecision(ctx, d1))
	require.NoError(t, store.SaveDecision(ctx, d2))
	require.NoError(t, store.SaveDecision(ctx, d3))

	// Query for auth file: should match d1 and d3 (global), but not d2 (billing)
	pack, err := store.QueryDecisionsForFiles(ctx, orgID, repoID, []string{"pkg/auth/service.go"}, 500)
	require.NoError(t, err)
	require.NotNil(t, pack)

	assert.Len(t, pack.Decisions, 2)
	assert.Equal(t, "ADR-0001", pack.Decisions[0].DecisionKey)
	assert.Equal(t, "ADR-0003", pack.Decisions[1].DecisionKey)

	// Verify prompt slice formatting
	promptSlice := pack.FormatPromptSlice()
	assert.Contains(t, promptSlice, "### ScanDrix Architectural Decisions & Trace Context")
	assert.Contains(t, promptSlice, "Argon2id")
	assert.Contains(t, promptSlice, "Bounded Context Timeouts")

	// Verify budget cut drops unpinned lower-confidence decisions
	smallPack, err := store.QueryDecisionsForFiles(ctx, orgID, repoID, []string{"pkg/auth/service.go"}, 25)
	require.NoError(t, err)
	// d1 is pinned so it remains; d3 is unpinned and dropped
	assert.Len(t, smallPack.Decisions, 1)
	assert.Equal(t, "ADR-0001", smallPack.Decisions[0].DecisionKey)
	assert.Equal(t, 1, smallPack.DroppedForBudget)
}

func TestTraceInvalidator_DetectsArchitecturalViolation(t *testing.T) {
	ctx := context.Background()
	store := trace.NewTraceStore()
	orgID := "org-1"
	repoID := "repo-1"

	adr := &trace.TraceDecision{
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-0015",
		Title:       "Enforce Parameterized SQL Queries",
		Decision:    "All database operations must use parameterized queries without raw sql string concatenation.",
		Rationale:   "Prevent SQL injection vectors across all storage repositories.",
		Status:      trace.StatusAccepted,
		Scope:       []string{"pkg/storage/**"},
	}
	require.NoError(t, store.SaveDecision(ctx, adr))

	invalidator := trace.NewTraceInvalidator(store)

	reviewID := uuid.New()
	workspaceID := uuid.New()

	// PR diff introducing raw SQL concatenation
	violatingPatch := &diff.FilePatch{
		NewPath: "pkg/storage/user_repo.go",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, Content: "func GetUser(id string) error {", OldLineNo: 10, NewLineNo: 10},
					{Type: diff.LineAddition, Content: `	q := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id)`, NewLineNo: 11},
					{Type: diff.LineAddition, Content: `	return db.Exec(q)`, NewLineNo: 12},
				},
			},
		},
	}

	report, err := invalidator.AuditPullRequest(ctx, reviewID, workspaceID, orgID, repoID, []*diff.FilePatch{violatingPatch})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Len(t, report.Violations, 1)
	assert.Equal(t, "ADR-0015", report.Violations[0].DecisionKey)
	assert.Equal(t, 11, report.Violations[0].LineNumber)
	assert.Contains(t, report.Violations[0].Violation, "Raw SQL string concatenation detected")

	require.Len(t, report.Findings, 1)
	finding := report.Findings[0]
	assert.Equal(t, "architecture", finding.Category)
	assert.Equal(t, models.SeverityHigh, finding.Severity)
	assert.Contains(t, finding.Title, "Architecture Invariant Violation [ADR-0015]")
}

func TestTraceInvalidator_HandlesSupersession(t *testing.T) {
	ctx := context.Background()
	store := trace.NewTraceStore()
	orgID := "org-1"
	repoID := "repo-1"

	oldADR := &trace.TraceDecision{
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-0001",
		Title:       "Use Redis for Cache",
		Decision:    "Central cache is Redis.",
		Status:      trace.StatusAccepted,
	}
	require.NoError(t, store.SaveDecision(ctx, oldADR))

	invalidator := trace.NewTraceInvalidator(store)

	adrPatch := &diff.FilePatch{
		NewPath: "docs/adr/ADR-0002-dragonfly-cache.md",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, Content: "# ADR-0002: Migrate to Dragonfly Cache"},
					{Type: diff.LineAddition, Content: "## Status: Accepted"},
					{Type: diff.LineAddition, Content: "supersedes: ADR-0001"},
				},
			},
		},
	}

	report, err := invalidator.AuditPullRequest(ctx, uuid.New(), uuid.New(), orgID, repoID, []*diff.FilePatch{adrPatch})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Contains(t, report.SupersededDecisions, "ADR-0001")

	// Verify old ADR status updated in store
	updatedOld, err := store.GetDecision(ctx, orgID, repoID, "ADR-0001")
	require.NoError(t, err)
	assert.Equal(t, trace.StatusSuperseded, updatedOld.Status)
	assert.Equal(t, "docs/adr/ADR-0002-dragonfly-cache.md", updatedOld.SupersededBy)
}

func TestInRepoADRParser_MarkdownAndJSON(t *testing.T) {
	parser := trace.NewInRepoADRParser()
	orgID := "org-x"
	repoID := "repo-y"

	mdContent := `
# ADR-0012: Enforce Pgx Driver
## Status: Accepted
## Scope: pkg/database/**, services/**

## Context
GORM causes connection pool exhaustion under high concurrency bursts.

## Decision
All database repositories MUST use pgxpool without ORM wrappers.

## Consequences
Queries must be written in parameterized SQL.
`

	patchMD := &diff.FilePatch{
		NewPath: "docs/adr/ADR-0012-enforce-pgx.md",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, Content: mdContent},
				},
			},
		},
	}

	jsonContent := `[
		{
			"decision_key": "trace-sec-jwt",
			"title": "Short Lived JWTs",
			"decision": "JWT expiration must not exceed 15 minutes.",
			"type": "security_policy",
			"status": "accepted",
			"scope": ["pkg/auth/**"]
		}
	]`

	patchJSON := &diff.FilePatch{
		NewPath: ".scandrix/trace/security.json",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, Content: jsonContent},
				},
			},
		},
	}

	decisions := parser.ScanPatchesForADRs([]*diff.FilePatch{patchMD, patchJSON}, orgID, repoID)
	require.Len(t, decisions, 2)

	d1 := decisions[0]
	assert.Equal(t, "ADR-0012-enforce-pgx", d1.DecisionKey)
	assert.Equal(t, "Enforce Pgx Driver", d1.Title)
	assert.Equal(t, trace.StatusAccepted, d1.Status)
	assert.Equal(t, "All database repositories MUST use pgxpool without ORM wrappers.", d1.Decision)
	assert.Contains(t, d1.Scope, "pkg/database/**")
	assert.Contains(t, d1.Scope, "services/**")

	d2 := decisions[1]
	assert.Equal(t, "trace-sec-jwt", d2.DecisionKey)
	assert.Equal(t, trace.DecisionSecurityPolicy, d2.Type)
	assert.True(t, d2.MatchesPath("pkg/auth/handler.go"))
	assert.False(t, d2.MatchesPath("pkg/billing/invoice.go"))
}
