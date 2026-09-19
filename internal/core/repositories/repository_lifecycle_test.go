package repositories_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Repository Lifecycle & Multi-Tenant Isolation Test Suite
// ============================================================================

// TestRepositoryLifecycle_MultiTenantIsolation verifies complete data isolation
// between distinct workspace tenants across all repository implementations.
func TestRepositoryLifecycle_MultiTenantIsolation(t *testing.T) {
	ctx := context.Background()

	wsTenantA := uuid.New()
	wsTenantB := uuid.New()
	wsTenantC := uuid.New()

	// 1. Tracked Repositories Multi-Tenant Isolation
	t.Run("TrackedRepository_Isolation", func(t *testing.T) {
		repo := NewMockTrackedRepoRepo()

		// Tenant A registers 4 repositories
		for i := 0; i < 4; i++ {
			r := &domain.TrackedRepository{
				TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantA},
				FullName:           fmt.Sprintf("tenant-a/service-%02d", i),
				DefaultBranch:      "main",
				IsActive:           true,
			}
			require.NoError(t, repo.Create(ctx, r))
		}

		// Tenant B registers 2 repositories
		for i := 0; i < 2; i++ {
			r := &domain.TrackedRepository{
				TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantB},
				FullName:           fmt.Sprintf("tenant-b/api-%02d", i),
				DefaultBranch:      "master",
				IsActive:           true,
			}
			require.NoError(t, repo.Create(ctx, r))
		}

		// Verify Tenant A sees exactly 4 repositories
		reposA, err := repo.FindByWorkspace(ctx, wsTenantA)
		require.NoError(t, err)
		assert.Len(t, reposA, 4)
		for _, r := range reposA {
			assert.Equal(t, wsTenantA, r.WorkspaceID)
			assert.Contains(t, r.FullName, "tenant-a/")
		}

		// Verify Tenant B sees exactly 2 repositories
		reposB, err := repo.FindByWorkspace(ctx, wsTenantB)
		require.NoError(t, err)
		assert.Len(t, reposB, 2)
		for _, r := range reposB {
			assert.Equal(t, wsTenantB, r.WorkspaceID)
			assert.Contains(t, r.FullName, "tenant-b/")
		}

		// Verify Tenant C sees 0 repositories
		reposC, err := repo.FindByWorkspace(ctx, wsTenantC)
		require.NoError(t, err)
		assert.Empty(t, reposC)
	})

	// 2. Teams Multi-Tenant Isolation
	t.Run("Teams_Isolation", func(t *testing.T) {
		repo := NewMockTeamRepo()

		teamA := &domain.Team{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantA},
			Name:               "Tenant A Backend Squad",
			Slug:               "tenant-a-backend",
			IsActive:           true,
		}
		require.NoError(t, repo.Create(ctx, teamA))

		teamB := &domain.Team{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantB},
			Name:               "Tenant B Frontend Squad",
			Slug:               "tenant-b-frontend",
			IsActive:           true,
		}
		require.NoError(t, repo.Create(ctx, teamB))

		teamsA, err := repo.FindByWorkspace(ctx, wsTenantA)
		require.NoError(t, err)
		assert.Len(t, teamsA, 1)
		assert.Equal(t, "Tenant A Backend Squad", teamsA[0].Name)

		teamsB, err := repo.FindByWorkspace(ctx, wsTenantB)
		require.NoError(t, err)
		assert.Len(t, teamsB, 1)
		assert.Equal(t, "Tenant B Frontend Squad", teamsB[0].Name)

		teamsC, err := repo.FindByWorkspace(ctx, wsTenantC)
		require.NoError(t, err)
		assert.Empty(t, teamsC)
	})

	// 3. CLI Device & Session Repository Isolation
	t.Run("CliDeviceAndSessionRepository_Isolation", func(t *testing.T) {
		deviceRepo := NewMockCliDeviceRepo()
		sessionRepo := NewMockCliSessionRepo()
		userA := uuid.New()
		userB := uuid.New()

		devA := &domain.CliDevice{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantA},
			UserID:             userA,
			DeviceIdentifier:   "DEVICE-ID-A-100",
			Hostname:           "tarun-macbook-pro",
			OS:                 "darwin",
			Arch:               "arm64",
			ClientVersion:      "v1.4.0",
			IsRevoked:          false,
			LastSeenAt:         time.Now().UTC(),
		}
		require.NoError(t, deviceRepo.Create(ctx, devA))

		devB := &domain.CliDevice{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantB},
			UserID:             userB,
			DeviceIdentifier:   "DEVICE-ID-B-200",
			Hostname:           "ci-runner-linux",
			OS:                 "linux",
			Arch:               "amd64",
			ClientVersion:      "v1.4.1",
			IsRevoked:          false,
			LastSeenAt:         time.Now().UTC(),
		}
		require.NoError(t, deviceRepo.Create(ctx, devB))

		// Device Lookup scoped by workspace
		byCodeA, err := deviceRepo.FindByDeviceIdentifier(ctx, wsTenantA, "DEVICE-ID-A-100")
		require.NoError(t, err)
		require.NotNil(t, byCodeA)
		assert.Equal(t, wsTenantA, byCodeA.WorkspaceID)

		byCodeCross, err := deviceRepo.FindByDeviceIdentifier(ctx, wsTenantB, "DEVICE-ID-A-100")
		require.NoError(t, err)
		assert.Nil(t, byCodeCross, "tenant B should not see tenant A device identifier")

		// Sessions Isolation
		now := time.Now().UTC()
		sessA := &domain.CliAuthSession{
			SessionCode: "sess_code_aaa_111",
			UserCode:    "USER-CODE-A",
			Status:      "PENDING",
			ExpiresAt:   now.Add(15 * time.Minute),
		}
		require.NoError(t, sessionRepo.Create(ctx, sessA))

		sessB := &domain.CliAuthSession{
			SessionCode: "sess_code_bbb_222",
			UserCode:    "USER-CODE-B",
			Status:      "PENDING",
			ExpiresAt:   now.Add(15 * time.Minute),
		}
		require.NoError(t, sessionRepo.Create(ctx, sessB))

		// Authorize sessions with respective tenant workspaces
		tokenA := "scandrix_auth_token_tenant_a"
		require.NoError(t, sessionRepo.Authorize(ctx, "USER-CODE-A", userA, wsTenantA, tokenA))

		tokenB := "scandrix_auth_token_tenant_b"
		require.NoError(t, sessionRepo.Authorize(ctx, "USER-CODE-B", userB, wsTenantB, tokenB))

		foundSessA, err := sessionRepo.FindByUserCode(ctx, "USER-CODE-A")
		require.NoError(t, err)
		require.NotNil(t, foundSessA)
		assert.Equal(t, &wsTenantA, foundSessA.WorkspaceID)
		assert.Equal(t, "AUTHORIZED", foundSessA.Status)

		foundSessB, err := sessionRepo.FindByUserCode(ctx, "USER-CODE-B")
		require.NoError(t, err)
		require.NotNil(t, foundSessB)
		assert.Equal(t, &wsTenantB, foundSessB.WorkspaceID)
		assert.Equal(t, "AUTHORIZED", foundSessB.Status)
	})

	// 4. Drixy Rules Repository Isolation
	t.Run("DrixyRulesRepository_Isolation", func(t *testing.T) {
		repo := NewMockDrixyRulesRepo()

		// Tenant A creates 4 rules
		for i := 0; i < 4; i++ {
			rule := &domain.DrixyRules{
				TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantA},
				RuleKey:            fmt.Sprintf("TENANT_A_RULE_%d", i),
				Name:               fmt.Sprintf("Tenant A Custom Rule %d", i),
				Category:           "SECURITY",
				Severity:           "CRITICAL",
				PromptInstructions: "Flag SQL injection and insecure deserialization",
				IsActive:           true,
				WeightMultiplier:   1.5,
			}
			require.NoError(t, repo.Create(ctx, rule))
		}

		// Tenant B creates 2 rules
		for i := 0; i < 2; i++ {
			rule := &domain.DrixyRules{
				TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantB},
				RuleKey:            fmt.Sprintf("TENANT_B_RULE_%d", i),
				Name:               fmt.Sprintf("Tenant B Style Rule %d", i),
				Category:           "STYLE",
				Severity:           "LOW",
				PromptInstructions: "Enforce naming conventions",
				IsActive:           true,
				WeightMultiplier:   0.8,
			}
			require.NoError(t, repo.Create(ctx, rule))
		}

		rulesA, err := repo.ListByWorkspace(ctx, wsTenantA)
		require.NoError(t, err)
		assert.Len(t, rulesA, 4)

		rulesB, err := repo.ListByWorkspace(ctx, wsTenantB)
		require.NoError(t, err)
		assert.Len(t, rulesB, 2)

		rulesC, err := repo.ListByWorkspace(ctx, wsTenantC)
		require.NoError(t, err)
		assert.Empty(t, rulesC)
	})

	// 5. Platform Pull Request Repository Isolation
	t.Run("PlatformPRRepository_Isolation", func(t *testing.T) {
		repo := NewMockPlatformPRRepo()
		repoIDA := uuid.New()
		repoIDB := uuid.New()

		prA := &domain.PullRequest{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantA},
			RepositoryID:       repoIDA,
			PullNumber:         101,
			Title:              "Add OAuth SSO integration",
			SourceBranch:       "feature/sso",
			TargetBranch:       "main",
			AuthorUsername:     "dev-alice",
			HeadSHA:            "a1b2c3d4e5f6",
			BaseSHA:            "f6e5d4c3b2a1",
			State:              "open",
		}
		require.NoError(t, repo.Upsert(ctx, prA))

		prB := &domain.PullRequest{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantB},
			RepositoryID:       repoIDB,
			PullNumber:         202,
			Title:              "Refactor database pooling",
			SourceBranch:       "fix/db-pool",
			TargetBranch:       "master",
			AuthorUsername:     "dev-bob",
			HeadSHA:            "112233445566",
			BaseSHA:            "665544332211",
			State:              "open",
		}
		require.NoError(t, repo.Upsert(ctx, prB))

		byNumA, err := repo.FindByNumber(ctx, wsTenantA, repoIDA, 101)
		require.NoError(t, err)
		require.NotNil(t, byNumA)
		assert.Equal(t, wsTenantA, byNumA.WorkspaceID)

		byNumB, err := repo.FindByNumber(ctx, wsTenantB, repoIDB, 202)
		require.NoError(t, err)
		require.NotNil(t, byNumB)
		assert.Equal(t, wsTenantB, byNumB.WorkspaceID)

		crossLookup, err := repo.FindByNumber(ctx, wsTenantB, repoIDA, 101)
		require.NoError(t, err)
		assert.Nil(t, crossLookup, "tenant B should not see tenant A PR")
	})

	// 6. Sandbox Lease Repository Isolation
	t.Run("SandboxLeaseRepository_Isolation", func(t *testing.T) {
		repo := NewMockSandboxLeaseRepo()
		revA := uuid.New()
		revB := uuid.New()

		leaseA := &repositories.SandboxLease{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantA},
			ReviewID:           revA,
			SandboxProvider:    "firecracker",
			ExternalLeaseID:    "sbx-tenant-a-101",
			Port:               8081,
			Status:             "ACTIVE",
			ExpiresAt:          time.Now().UTC().Add(30 * time.Minute),
		}
		require.NoError(t, repo.AcquireLease(ctx, leaseA))

		leaseB := &repositories.SandboxLease{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantB},
			ReviewID:           revB,
			SandboxProvider:    "gvisor",
			ExternalLeaseID:    "sbx-tenant-b-202",
			Port:               8082,
			Status:             "ACTIVE",
			ExpiresAt:          time.Now().UTC().Add(15 * time.Minute),
		}
		require.NoError(t, repo.AcquireLease(ctx, leaseB))

		foundA, err := repo.FindByReviewID(ctx, wsTenantA, revA)
		require.NoError(t, err)
		require.NotNil(t, foundA)
		assert.Equal(t, "sbx-tenant-a-101", foundA.ExternalLeaseID)

		foundCross, err := repo.FindByReviewID(ctx, wsTenantB, revA)
		require.NoError(t, err)
		assert.Nil(t, foundCross, "tenant B should not see tenant A sandbox lease")
	})

	// 7. Token Usage Repository Isolation
	t.Run("TokenUsageRepository_Isolation", func(t *testing.T) {
		repo := NewMockTokenUsageRepo()
		now := time.Now().UTC()

		for i := 0; i < 5; i++ {
			usageA := &domain.TokenUsageRecord{
				WorkspaceID:      wsTenantA,
				ModelName:        "claude-3-7-sonnet",
				PromptTokens:     1000,
				CompletionTokens: 200,
				TotalTokens:      1200,
				EstimatedCostUSD: 0.010,
				OperationType:    "code_review",
				RecordedAt:       now,
			}
			require.NoError(t, repo.Record(ctx, usageA))
		}

		for i := 0; i < 3; i++ {
			usageB := &domain.TokenUsageRecord{
				WorkspaceID:      wsTenantB,
				ModelName:        "gpt-4o",
				PromptTokens:     2000,
				CompletionTokens: 500,
				TotalTokens:      2500,
				EstimatedCostUSD: 0.020,
				OperationType:    "ast_analysis",
				RecordedAt:       now,
			}
			require.NoError(t, repo.Record(ctx, usageB))
		}

		spendA, err := repo.GetMonthlySpend(ctx, wsTenantA, now.Year(), now.Month())
		require.NoError(t, err)
		assert.InDelta(t, 0.050, spendA, 0.0001)

		spendB, err := repo.GetMonthlySpend(ctx, wsTenantB, now.Year(), now.Month())
		require.NoError(t, err)
		assert.InDelta(t, 0.060, spendB, 0.0001)

		spendC, err := repo.GetMonthlySpend(ctx, wsTenantC, now.Year(), now.Month())
		require.NoError(t, err)
		assert.Equal(t, 0.0, spendC)
	})

	// 8. User Assignment Repository Isolation
	t.Run("UserAssignmentRepository_Isolation", func(t *testing.T) {
		repo := NewMockUserAssignmentRepo()
		user := uuid.New()
		repoA := uuid.New()
		repoB := uuid.New()

		assignA := &repositories.UserRepositoryAssignment{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantA},
			UserID:             user,
			RepositoryID:       repoA,
			Role:               "ADMIN",
			IsActive:           true,
		}
		require.NoError(t, repo.Assign(ctx, assignA))

		assignB := &repositories.UserRepositoryAssignment{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsTenantB},
			UserID:             user,
			RepositoryID:       repoB,
			Role:               "REVIEWER",
			IsActive:           true,
		}
		require.NoError(t, repo.Assign(ctx, assignB))

		userAssignsA, err := repo.ListByUser(ctx, wsTenantA, user)
		require.NoError(t, err)
		assert.Len(t, userAssignsA, 1)
		assert.Equal(t, repoA, userAssignsA[0].RepositoryID)

		userAssignsB, err := repo.ListByUser(ctx, wsTenantB, user)
		require.NoError(t, err)
		assert.Len(t, userAssignsB, 1)
		assert.Equal(t, repoB, userAssignsB[0].RepositoryID)
	})
}

// TestRepositoryLifecycle_PaginationEdgeCases tests edge cases in pagination mathematics:
// offset beyond length, zero limit, high page index, and sorting stability.
func TestRepositoryLifecycle_PaginationEdgeCases(t *testing.T) {
	ctx := context.Background()
	repo := NewMockWorkspaceRepo()

	const totalItems = 75
	for i := 0; i < totalItems; i++ {
		ws := &domain.Workspace{
			Name:   fmt.Sprintf("Workspace-%03d", i),
			Slug:   fmt.Sprintf("workspace-%03d", i),
			Status: "ACTIVE",
			Tier:   "ENTERPRISE",
		}
		require.NoError(t, repo.Create(ctx, ws))
	}

	allWorkspaces, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, allWorkspaces, totalItems)

	// Sort workspaces by name to ensure stable pagination ordering
	sort.Slice(allWorkspaces, func(i, j int) bool {
		return allWorkspaces[i].Name < allWorkspaces[j].Name
	})

	// Simulate pagination slices
	paginate := func(items []*domain.Workspace, page, limit int) []*domain.Workspace {
		if limit <= 0 {
			limit = 10
		}
		if page <= 0 {
			page = 1
		}
		start := (page - 1) * limit
		if start >= len(items) {
			return []*domain.Workspace{}
		}
		end := start + limit
		if end > len(items) {
			end = len(items)
		}
		return items[start:end]
	}

	// Page 1, limit 20
	p1 := paginate(allWorkspaces, 1, 20)
	assert.Len(t, p1, 20)
	assert.Equal(t, "Workspace-000", p1[0].Name)
	assert.Equal(t, "Workspace-019", p1[19].Name)

	// Page 4, limit 20 (75 total items -> items 60..74 = 15 items)
	p4 := paginate(allWorkspaces, 4, 20)
	assert.Len(t, p4, 15)
	assert.Equal(t, "Workspace-060", p4[0].Name)
	assert.Equal(t, "Workspace-074", p4[14].Name)

	// Page 5, limit 20 (Out of bounds -> empty)
	p5 := paginate(allWorkspaces, 5, 20)
	assert.Empty(t, p5)

	// Zero or negative limits fallback gracefully
	pNeg := paginate(allWorkspaces, 1, -5)
	assert.Len(t, pNeg, 10, "negative limit should default to 10")
}

// TestRepositoryLifecycle_ConcurrentWriteSafety tests high-contention concurrent creates and updates.
func TestRepositoryLifecycle_ConcurrentWriteSafety(t *testing.T) {
	ctx := context.Background()
	repo := NewMockDrixyRulesRepo()
	wsID := uuid.New()

	const concurrentGoroutines = 30
	const rulesPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(concurrentGoroutines)

	for g := 0; g < concurrentGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()
			for r := 0; r < rulesPerGoroutine; r++ {
				ruleKey := fmt.Sprintf("RULE-%d-%d", gID, r)
				rule := &domain.DrixyRules{
					TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
					RuleKey:            ruleKey,
					Name:               fmt.Sprintf("Rule %s", ruleKey),
					Category:           "PERFORMANCE",
					Severity:           "MEDIUM",
					PromptInstructions: "Profile loop iterations",
					IsActive:           true,
					WeightMultiplier:   1.0,
				}
				require.NoError(t, repo.Create(ctx, rule))
			}
		}(g)
	}

	wg.Wait()

	rules, err := repo.ListByWorkspace(ctx, wsID)
	require.NoError(t, err)
	assert.Equal(t, concurrentGoroutines*rulesPerGoroutine, len(rules))

	// Concurrent verification and retrieval
	wg.Add(len(rules))
	for _, r := range rules {
		go func(ruleID uuid.UUID) {
			defer wg.Done()
			found, err := repo.FindByID(ctx, ruleID)
			assert.NoError(t, err)
			assert.NotNil(t, found)
			assert.Equal(t, wsID, found.WorkspaceID)
		}(r.ID)
	}
	wg.Wait()
}

// TestRepositoryLifecycle_SoftDeleteAndRestore tests soft deletion lifecycles.
func TestRepositoryLifecycle_SoftDeleteAndRestore(t *testing.T) {
	ctx := context.Background()
	repo := NewMockWorkspaceRepo()

	ws := &domain.Workspace{
		Name:   "Ephemeral-Workspace",
		Slug:   "ephemeral-workspace",
		Status: "ACTIVE",
		Tier:   "DEVELOPER",
	}
	require.NoError(t, repo.Create(ctx, ws))

	// Verify active
	found, err := repo.FindByID(ctx, ws.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "ACTIVE", found.Status)

	// Soft-deactivate
	found.Status = "SUSPENDED"
	require.NoError(t, repo.Update(ctx, found))

	deactivated, err := repo.FindByID(ctx, ws.ID)
	require.NoError(t, err)
	require.NotNil(t, deactivated)
	assert.Equal(t, "SUSPENDED", deactivated.Status)

	// Restore
	deactivated.Status = "ACTIVE"
	require.NoError(t, repo.Update(ctx, deactivated))

	restored, err := repo.FindByID(ctx, ws.ID)
	require.NoError(t, err)
	require.NotNil(t, restored)
	assert.Equal(t, "ACTIVE", restored.Status)

	// Hard delete
	require.NoError(t, repo.Delete(ctx, ws.ID))
	deleted, err := repo.FindByID(ctx, ws.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

// TestRepositoryLifecycle_BatchAssignmentManagement tests bulk user repository assignments and access control.
func TestRepositoryLifecycle_BatchAssignmentManagement(t *testing.T) {
	ctx := context.Background()
	repo := NewMockUserAssignmentRepo()
	wsID := uuid.New()
	userID := uuid.New()

	const numRepos = 25
	var repoIDs []uuid.UUID
	for i := 0; i < numRepos; i++ {
		rID := uuid.New()
		repoIDs = append(repoIDs, rID)
		assignment := &repositories.UserRepositoryAssignment{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			UserID:             userID,
			RepositoryID:       rID,
			Role:               "ADMIN",
			IsActive:           true,
		}
		require.NoError(t, repo.Assign(ctx, assignment))
	}

	// Verify all assigned
	assignments, err := repo.ListByUser(ctx, wsID, userID)
	require.NoError(t, err)
	assert.Len(t, assignments, numRepos)

	// Revoke half
	for i := 0; i < numRepos/2; i++ {
		err := repo.Revoke(ctx, wsID, userID, repoIDs[i])
		require.NoError(t, err)
	}

	remaining, err := repo.ListByUser(ctx, wsID, userID)
	require.NoError(t, err)
	assert.Len(t, remaining, numRepos-(numRepos/2))
}

// TestRepositoryLifecycle_ReviewFindingsCascade exercises pull request reviews and hierarchical findings.
func TestRepositoryLifecycle_ReviewFindingsCascade(t *testing.T) {
	ctx := context.Background()
	reviewRepo := NewMockReviewRepo()
	findingRepo := NewMockFindingRepo()

	repoID := uuid.New()
	prReview := &domain.PullRequestReview{
		RepositoryID: repoID,
		PullNumber:   42,
		HeadSHA:      "git-commit-sha-4242",
		BaseSHA:      "git-commit-base-4242",
		State:        "IN_PROGRESS",
	}
	require.NoError(t, reviewRepo.Create(ctx, prReview))
	assert.NotEqual(t, uuid.Nil, prReview.ID)

	// Add findings to review
	const numFindings = 10
	for i := 0; i < numFindings; i++ {
		finding := &domain.CodeFinding{
			ReviewID:     prReview.ID,
			RepositoryID: repoID,
			FilePath:     fmt.Sprintf("internal/pkg/file_%02d.go", i),
			LineStart:    10 + i,
			LineEnd:      12 + i,
			Severity:     "HIGH",
			RuleID:       fmt.Sprintf("RULE-%03d", i),
			Category:     "SECURITY",
			Message:      fmt.Sprintf("Potential nil dereference in file_%02d.go", i),
		}
		require.NoError(t, findingRepo.Create(ctx, finding))
	}

	// Verify findings retrieved by review ID
	findings, err := findingRepo.FindByReviewID(ctx, prReview.ID)
	require.NoError(t, err)
	assert.Len(t, findings, numFindings)

	// Sort findings by line start to verify sequential mapping
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].LineStart < findings[j].LineStart
	})

	for idx, f := range findings {
		assert.Equal(t, prReview.ID, f.ReviewID)
		assert.Equal(t, fmt.Sprintf("internal/pkg/file_%02d.go", idx), f.FilePath)
		assert.Equal(t, "HIGH", f.Severity)
	}

	// Lookup reviews by repository
	reviews, err := reviewRepo.FindByRepository(ctx, repoID)
	require.NoError(t, err)
	assert.Len(t, reviews, 1)
	assert.Equal(t, 42, reviews[0].PullNumber)
}

// TestRepositoryLifecycle_CliSessionPurgeExpired verifies atomic cleanup of stale CLI auth sessions.
func TestRepositoryLifecycle_CliSessionPurgeExpired(t *testing.T) {
	ctx := context.Background()
	repo := NewMockCliSessionRepo()
	now := time.Now().UTC()

	// 10 expired sessions
	for i := 0; i < 10; i++ {
		sess := &domain.CliAuthSession{
			SessionCode: fmt.Sprintf("expired-sess-%02d", i),
			UserCode:    fmt.Sprintf("EXP-%02d", i),
			Status:      "PENDING",
			ExpiresAt:   now.Add(-1 * time.Hour),
		}
		require.NoError(t, repo.Create(ctx, sess))
	}

	// 5 active sessions
	for i := 0; i < 5; i++ {
		sess := &domain.CliAuthSession{
			SessionCode: fmt.Sprintf("active-sess-%02d", i),
			UserCode:    fmt.Sprintf("ACT-%02d", i),
			Status:      "PENDING",
			ExpiresAt:   now.Add(1 * time.Hour),
		}
		require.NoError(t, repo.Create(ctx, sess))
	}

	purgedCount, err := repo.PurgeExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), purgedCount)

	// Confirm active sessions remain
	for i := 0; i < 5; i++ {
		found, err := repo.FindBySessionCode(ctx, fmt.Sprintf("active-sess-%02d", i))
		require.NoError(t, err)
		assert.NotNil(t, found)
	}

	// Confirm expired are gone
	for i := 0; i < 10; i++ {
		found, err := repo.FindBySessionCode(ctx, fmt.Sprintf("expired-sess-%02d", i))
		require.NoError(t, err)
		assert.Nil(t, found)
	}
}

// TestRepositoryLifecycle_CliDeviceRevocation verifies revocation behavior.
func TestRepositoryLifecycle_CliDeviceRevocation(t *testing.T) {
	ctx := context.Background()
	repo := NewMockCliDeviceRepo()
	wsID := uuid.New()
	userID := uuid.New()

	dev := &domain.CliDevice{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		UserID:             userID,
		DeviceIdentifier:   "REVOKE-TEST-DEV-1",
		Hostname:           "dev-laptop",
		OS:                 "linux",
		Arch:               "amd64",
		ClientVersion:      "v1.0.0",
		IsRevoked:          false,
		LastSeenAt:         time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, dev))

	// Update last seen
	require.NoError(t, repo.UpdateLastSeen(ctx, wsID, dev.ID, "v1.1.0"))
	updated, err := repo.FindByID(ctx, wsID, dev.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "v1.1.0", updated.ClientVersion)
	assert.False(t, updated.IsRevoked)

	// Revoke device
	require.NoError(t, repo.Revoke(ctx, wsID, dev.ID))
	revoked, err := repo.FindByID(ctx, wsID, dev.ID)
	require.NoError(t, err)
	require.NotNil(t, revoked)
	assert.True(t, revoked.IsRevoked)

	// Cross workspace revocation fails
	err = repo.Revoke(ctx, uuid.New(), dev.ID)
	assert.Error(t, err)
}

// TestRepositoryLifecycle_ConcurrentLeaseAcquisition verifies race-free sandbox lease allocation.
func TestRepositoryLifecycle_ConcurrentLeaseAcquisition(t *testing.T) {
	ctx := context.Background()
	repo := NewMockSandboxLeaseRepo()
	wsID := uuid.New()

	const concurrentWorkers = 20
	var wg sync.WaitGroup
	var acquiredCount int64

	wg.Add(concurrentWorkers)
	for i := 0; i < concurrentWorkers; i++ {
		go func(workerID int) {
			defer wg.Done()
			reviewID := uuid.New()
			lease := &repositories.SandboxLease{
				TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
				ReviewID:           reviewID,
				SandboxProvider:    "firecracker",
				ExternalLeaseID:    fmt.Sprintf("sbx-%d", workerID),
				Port:               8000 + workerID,
				Status:             "ACTIVE",
				ExpiresAt:          time.Now().UTC().Add(10 * time.Minute),
			}
			err := repo.AcquireLease(ctx, lease)
			if err == nil {
				atomic.AddInt64(&acquiredCount, 1)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(concurrentWorkers), acquiredCount)
}
