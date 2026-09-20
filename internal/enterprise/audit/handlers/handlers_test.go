// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeReviewConfigHandler_DiffAndValidation(t *testing.T) {
	h := NewCodeReviewConfigHandler()

	oldCfg := map[string]any{
		"auto_approve": false,
		"threshold":    "CRITICAL",
		"max_turns":    10,
	}
	newCfg := map[string]any{
		"auto_approve": true,
		"threshold":    "HIGH",
		"max_turns":    10,
		"new_field":    "enabled",
	}

	diffs := h.ComputeConfigDiff(oldCfg, newCfg)
	assert.Len(t, diffs, 3) // auto_approve, threshold, new_field

	// Test validation
	ctx := context.Background()
	validEv := audit.EnterpriseLogEvent{
		Target:  audit.TargetContext{TargetEntityID: "cfg-1"},
		Action:  "UPDATE",
		Changes: diffs,
	}
	err := h.HandleEvent(ctx, validEv)
	require.NoError(t, err)

	invalidEv := audit.EnterpriseLogEvent{
		Target: audit.TargetContext{TargetEntityID: "cfg-1"},
		Action: "UPDATE",
	}
	err = h.HandleEvent(ctx, invalidEv)
	assert.Error(t, err)
}

func TestRulesConfigHandler_Validation(t *testing.T) {
	h := NewRulesConfigHandler()
	ctx := context.Background()

	err := h.HandleEvent(ctx, audit.EnterpriseLogEvent{
		Target: audit.TargetContext{TargetEntityID: "rule-123"},
		Action: "CREATE",
	})
	require.NoError(t, err)

	err = h.HandleEvent(ctx, audit.EnterpriseLogEvent{
		Target: audit.TargetContext{TargetEntityID: "rule-123"},
		Action: "INVALID_ACTION",
	})
	assert.Error(t, err)
}

func TestUserHandlers_Validation(t *testing.T) {
	ctx := context.Background()

	statusH := NewUserStatusHandler()
	err := statusH.HandleEvent(ctx, audit.EnterpriseLogEvent{Target: audit.TargetContext{TargetEntityID: "usr-1"}})
	require.NoError(t, err)

	inviteH := NewUserInviteHandler()
	err = inviteH.HandleEvent(ctx, audit.EnterpriseLogEvent{Target: audit.TargetContext{TargetEntityID: "inv-1"}})
	require.NoError(t, err)

	roleH := NewUserRoleChangeHandler()
	err = roleH.HandleEvent(ctx, audit.EnterpriseLogEvent{Target: audit.TargetContext{TargetEntityID: "usr-1"}})
	require.NoError(t, err)

	repoAccessH := NewUserRepoAccessHandler()
	err = repoAccessH.HandleEvent(ctx, audit.EnterpriseLogEvent{Target: audit.TargetContext{TargetEntityID: "usr-1"}})
	require.NoError(t, err)
}

func TestUnifiedLogHandler_TimelineAndExports(t *testing.T) {
	repo := audit.NewMemoryAuditRepository("unified-test-key")
	ctx := context.Background()
	orgID := uuid.New()

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		_, err := repo.AppendLog(ctx, audit.EnterpriseLogEvent{
			Category: audit.CategoryOrgSettings,
			Action:   "UPDATE",
			Actor: audit.ActorContext{
				UserID:   "admin-1",
				Email:    "admin@scandrix.dev",
				ClientIP: "10.0.0.1",
			},
			Target: audit.TargetContext{
				OrganizationID: orgID,
				TargetEntityID: "settings",
				TargetType:     "org_settings",
			},
			Timestamp: now,
		})
		require.NoError(t, err)
	}

	unified := NewUnifiedLogHandler(repo)
	filter := audit.AuditLogFilter{OrganizationID: &orgID}

	// 1. Timeline
	timeline, err := unified.GetTimeline(ctx, filter)
	require.NoError(t, err)
	require.Len(t, timeline, 1)
	assert.Equal(t, 3, timeline[0].ActionCount)
	assert.Equal(t, "admin@scandrix.dev", timeline[0].Actor)

	// 2. CSV Export
	csvBytes, err := unified.ExportCSV(ctx, filter)
	require.NoError(t, err)
	assert.Contains(t, string(csvBytes), "Event ID,Timestamp (UTC),Category")
	assert.Contains(t, string(csvBytes), "admin@scandrix.dev")

	// 3. JSON Lines Export
	ndjsonBytes, err := unified.ExportJSONLines(ctx, filter)
	require.NoError(t, err)
	assert.Contains(t, string(ndjsonBytes), "admin@scandrix.dev")
	assert.Contains(t, string(ndjsonBytes), "ORG_SETTINGS")
}
