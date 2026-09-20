// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package audit

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryAuditRepository_AppendAndQuery(t *testing.T) {
	repo := NewMemoryAuditRepository("test-secret-key")
	ctx := context.Background()

	orgID := uuid.New()
	wsID := uuid.New()

	ev1 := EnterpriseLogEvent{
		Category: CategoryCodeReviewConfig,
		Action:   "UPDATE",
		Actor: ActorContext{
			UserID: "user-123",
			Email:  "admin@scandrix.dev",
		},
		Target: TargetContext{
			OrganizationID: orgID,
			WorkspaceID:    wsID,
			TargetEntityID: "config-1",
		},
	}

	ev2 := EnterpriseLogEvent{
		Category: CategoryDrixyRules,
		Action:   "CREATE",
		Actor: ActorContext{
			UserID: "user-456",
			Email:  "dev@scandrix.dev",
		},
		Target: TargetContext{
			OrganizationID: orgID,
			WorkspaceID:    wsID,
			TargetEntityID: "rule-99",
		},
	}

	res1, err := repo.AppendLog(ctx, ev1)
	require.NoError(t, err)
	assert.NotEmpty(t, res1.Hash)

	res2, err := repo.AppendLog(ctx, ev2)
	require.NoError(t, err)
	assert.Equal(t, res1.Hash, res2.PrevHash) // Hash chaining!

	// Query by Org
	logs, total, err := repo.QueryLogs(ctx, AuditLogFilter{OrganizationID: &orgID})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, logs, 2)

	// Query by Category
	cat := CategoryDrixyRules
	logs, total, err = repo.QueryLogs(ctx, AuditLogFilter{OrganizationID: &orgID, Category: &cat})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "rule-99", logs[0].Target.TargetEntityID)
}

func TestMemoryAuditRepository_VerifyChainIntegrity(t *testing.T) {
	repo := NewMemoryAuditRepository("test-secret-key")
	ctx := context.Background()
	orgID := uuid.New()

	// Append 3 records
	for i := 0; i < 3; i++ {
		_, err := repo.AppendLog(ctx, EnterpriseLogEvent{
			Category: CategoryUserStatus,
			Action:   "UPDATE",
			Actor:    ActorContext{UserID: "admin", Email: "admin@scandrix.dev"},
			Target:   TargetContext{OrganizationID: orgID, TargetEntityID: uuid.New().String()},
		})
		require.NoError(t, err)
	}

	// Verify intact chain
	valid, corruptID, err := repo.VerifyChainIntegrity(ctx, orgID)
	require.NoError(t, err)
	assert.True(t, valid)
	assert.Nil(t, corruptID)

	// Deliberately tamper with record 1
	repo.mu.Lock()
	repo.logs[1].Actor.Email = "attacker@scandrix.dev"
	repo.mu.Unlock()

	// Verify chain detects tampering
	valid, corruptID, err = repo.VerifyChainIntegrity(ctx, orgID)
	assert.False(t, valid)
	assert.NotNil(t, corruptID)
	assert.Error(t, err)
}
