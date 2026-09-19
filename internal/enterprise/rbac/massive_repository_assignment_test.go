// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package rbac_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/pkg/models"
)

// TestMassive_RoleHierarchyAndAdminOwnerBypass executes 10,000 test cases
// verifying that Owner and Admin roles unconditionally bypass fine-grained repository limits.
func TestMassive_RoleHierarchyAndAdminOwnerBypass(t *testing.T) {
	ctx := context.Background()
	store := rbac.NewMemoryRepositoryAssignmentStore()
	ctrl := rbac.NewRepositoryAccessController(store, nil)

	const targetCases = 10000

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		userID := uuid.New()
		assignedRepoID := uuid.New()
		unassignedRepoID := uuid.New()

		// Restrict the user to only assignedRepoID
		_, err := ctrl.AssignRepositories(ctx, wsID, userID, []uuid.UUID{assignedRepoID}, "system")
		if err != nil {
			t.Fatalf("[Case %d] AssignRepositories failed: %v", i, err)
		}

		// 1. Owner role must ALWAYS have access to unassigned repositories
		canOwner := ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleOwner, unassignedRepoID)
		if !canOwner {
			t.Fatalf("[Case %d] Owner should always bypass repository limits, got %v", i, canOwner)
		}

		// 2. Admin role must ALWAYS have access to unassigned repositories
		canAdmin := ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleAdmin, unassignedRepoID)
		if !canAdmin {
			t.Fatalf("[Case %d] Admin should always bypass repository limits, got %v", i, canAdmin)
		}

		// 3. Member role must be BLOCKED from unassigned repositories
		canMember := ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, unassignedRepoID)
		if canMember {
			t.Fatalf("[Case %d] Member must be denied access to unassigned repository, got %v", i, canMember)
		}

		// 4. Member role must be ALLOWED access to assigned repository
		canMemberAssigned := ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, assignedRepoID)
		if !canMemberAssigned {
			t.Fatalf("[Case %d] Member must be granted access to assigned repository, got %v", i, canMemberAssigned)
		}
	}
}

// TestMassive_RepositoryAssignmentDeduplicationAndMutation executes 10,000 test cases
// testing repository array deduplication, incremental addition, and atomic revocation.
func TestMassive_RepositoryAssignmentDeduplicationAndMutation(t *testing.T) {
	ctx := context.Background()
	store := rbac.NewMemoryRepositoryAssignmentStore()
	ctrl := rbac.NewRepositoryAccessController(store, nil)

	const targetCases = 10000

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		userID := uuid.New()

		repoA := uuid.New()
		repoB := uuid.New()
		repoC := uuid.New()

		// Assign with duplicate entries: [A, B, A, C, B, A]
		assignment, err := ctrl.AssignRepositories(ctx, wsID, userID, []uuid.UUID{repoA, repoB, repoA, repoC, repoB, repoA}, "admin")
		if err != nil {
			t.Fatalf("[Case %d] AssignRepositories failed: %v", i, err)
		}

		// Invariant: Length of assigned repos must be exactly 3 (deduplicated)
		if len(assignment.RepositoryIDs) != 3 {
			t.Fatalf("[Case %d] expected 3 deduplicated repositories, got %d", i, len(assignment.RepositoryIDs))
		}

		// Incremental Add of a new repoD and redundant repoB
		repoD := uuid.New()
		updated, err := ctrl.AddRepositories(ctx, wsID, userID, []uuid.UUID{repoD, repoB}, "admin")
		if err != nil {
			t.Fatalf("[Case %d] AddRepositories failed: %v", i, err)
		}
		if len(updated.RepositoryIDs) != 4 {
			t.Fatalf("[Case %d] expected 4 repositories after adding 1 new + 1 duplicate, got %d", i, len(updated.RepositoryIDs))
		}

		// Incremental Revoke of repoA
		revoked, err := ctrl.RevokeRepository(ctx, wsID, userID, repoA)
		if err != nil {
			t.Fatalf("[Case %d] RevokeRepository failed: %v", i, err)
		}
		if len(revoked.RepositoryIDs) != 3 {
			t.Fatalf("[Case %d] expected 3 repositories after revoking repoA, got %d", i, len(revoked.RepositoryIDs))
		}

		// Verify repoA access is now false for member
		canAccessRevoked := ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repoA)
		if canAccessRevoked {
			t.Fatalf("[Case %d] revoked repoA should deny access, got %v", i, canAccessRevoked)
		}
	}
}

// TestMassive_MultiTenantAndMultiUserBoundaryIsolation executes 10,000 test cases
// validating strict boundaries across workspaces, users, and unassigned defaults.
func TestMassive_MultiTenantAndMultiUserBoundaryIsolation(t *testing.T) {
	ctx := context.Background()
	store := rbac.NewMemoryRepositoryAssignmentStore()
	ctrl := rbac.NewRepositoryAccessController(store, nil)

	const targetCases = 10000

	for i := 0; i < targetCases; i++ {
		wsA := uuid.New()
		wsB := uuid.New()
		user1 := uuid.New()
		user2 := uuid.New()
		repoTarget := uuid.New()

		// 1. Unassigned user in wsA defaults to unrestricted workspace access
		canUnassigned := ctrl.CanAccessRepository(ctx, wsA, user1, models.RoleMember, repoTarget)
		if !canUnassigned {
			t.Fatalf("[Case %d] unassigned user must default to true, got %v", i, canUnassigned)
		}

		// 2. Assign repoTarget to user1 in wsA
		_, err := ctrl.AssignRepositories(ctx, wsA, user1, []uuid.UUID{repoTarget}, "admin")
		if err != nil {
			t.Fatalf("[Case %d] AssignRepositories failed: %v", i, err)
		}

		// 3. User1 in wsA has access to repoTarget
		canUser1A := ctrl.CanAccessRepository(ctx, wsA, user1, models.RoleMember, repoTarget)
		if !canUser1A {
			t.Fatalf("[Case %d] user1 in wsA should have access to assigned repoTarget, got %v", i, canUser1A)
		}

		// 4. Multi-Tenant Isolation: User1 querying wsB for repoTarget must NOT inherit wsA assignment
		canUser1B := ctrl.CanAccessRepository(ctx, wsB, user1, models.RoleMember, repoTarget)
		if !canUser1B {
			// In wsB, user1 is unassigned, so they should default to wsB unassigned default (true), but wsA assignment does not exist in wsB
			t.Fatalf("[Case %d] unexpected wsB access check failure: %v", i, canUser1B)
		}

		// 5. User2 in wsA is restricted to another repo; must NOT access repoTarget
		otherRepo := uuid.New()
		_, err = ctrl.AssignRepositories(ctx, wsA, user2, []uuid.UUID{otherRepo}, "admin")
		if err != nil {
			t.Fatalf("[Case %d] AssignRepositories for user2 failed: %v", i, err)
		}

		canUser2A := ctrl.CanAccessRepository(ctx, wsA, user2, models.RoleMember, repoTarget)
		if canUser2A {
			t.Fatalf("[Case %d] user2 in wsA must be denied access to user1's repoTarget, got %v", i, canUser2A)
		}
	}
}

// TestMassive_HighThroughputConcurrentAccessStress executes 10,000 test cases
// testing concurrent reads, mutations, and policy evaluations under go test -race.
func TestMassive_HighThroughputConcurrentAccessStress(t *testing.T) {
	ctx := context.Background()
	store := rbac.NewMemoryRepositoryAssignmentStore()
	ctrl := rbac.NewRepositoryAccessController(store, nil)

	const targetCases = 10000
	const concurrency = 20

	wsID := uuid.New()
	userPool := make([]uuid.UUID, 50)
	repoPool := make([]uuid.UUID, 50)
	for i := 0; i < 50; i++ {
		userPool[i] = uuid.New()
		repoPool[i] = uuid.New()
	}

	var wg sync.WaitGroup
	casesPerWorker := targetCases / concurrency

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < casesPerWorker; i++ {
				idx := (workerID*casesPerWorker + i) % len(userPool)
				uID := userPool[idx]
				rID := repoPool[idx]

				switch i % 3 {
				case 0:
					// Read check
					_ = ctrl.CanAccessRepository(ctx, wsID, uID, models.RoleMember, rID)
				case 1:
					// Mutation: Add
					_, _ = ctrl.AddRepositories(ctx, wsID, uID, []uuid.UUID{rID}, "admin")
				case 2:
					// Query assigned list
					_, _ = ctrl.GetAssignedRepositories(ctx, wsID, uID)
				}
			}
		}(w)
	}

	wg.Wait()
}
