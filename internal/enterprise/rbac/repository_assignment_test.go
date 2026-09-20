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

func TestRepositoryAssignment_Lifecycle(t *testing.T) {
	store := rbac.NewMemoryRepositoryAssignmentStore()
	engine := rbac.NewPolicyEngine()
	ctrl := rbac.NewRepositoryAccessController(store, engine)
	ctx := context.Background()

	wsID := uuid.New()
	userID := uuid.New()
	repoA := uuid.New()
	repoB := uuid.New()
	repoC := uuid.New()

	// 1. Initial state: no assignments -> member can access repoA (default open workspace)
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repoA) {
		t.Fatal("Expected member without explicit restrictions to access repoA")
	}

	// 2. Assign repoA and repoB
	assignment, err := ctrl.AssignRepositories(ctx, wsID, userID, []uuid.UUID{repoA, repoB}, "admin@corp.com")
	if err != nil {
		t.Fatalf("AssignRepositories failed: %v", err)
	}
	if len(assignment.RepositoryIDs) != 2 {
		t.Fatalf("Expected 2 assigned repos, got %d", len(assignment.RepositoryIDs))
	}

	// 3. User can access repoA and repoB, but CANNOT access repoC
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repoA) {
		t.Fatal("Expected user to access assigned repoA")
	}
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repoB) {
		t.Fatal("Expected user to access assigned repoB")
	}
	if ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repoC) {
		t.Fatal("Expected user to be DENIED access to unassigned repoC")
	}

	// 4. Admin and Owner override: can access repoC regardless of user restrictions
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleAdmin, repoC) {
		t.Fatal("Expected Admin to access unassigned repoC")
	}
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleOwner, repoC) {
		t.Fatal("Expected Owner to access unassigned repoC")
	}

	// 5. Add repoC
	assignment, err = ctrl.AddRepositories(ctx, wsID, userID, []uuid.UUID{repoC}, "admin@corp.com")
	if err != nil {
		t.Fatalf("AddRepositories failed: %v", err)
	}
	if len(assignment.RepositoryIDs) != 3 {
		t.Fatalf("Expected 3 assigned repos, got %d", len(assignment.RepositoryIDs))
	}
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repoC) {
		t.Fatal("Expected user to access newly added repoC")
	}

	// 6. Revoke repoA
	assignment, err = ctrl.RevokeRepository(ctx, wsID, userID, repoA)
	if err != nil {
		t.Fatalf("RevokeRepository failed: %v", err)
	}
	if len(assignment.RepositoryIDs) != 2 {
		t.Fatalf("Expected 2 remaining assigned repos, got %d", len(assignment.RepositoryIDs))
	}
	if ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repoA) {
		t.Fatal("Expected revoked repoA to be denied")
	}

	// 7. Get assigned repos
	repos, err := ctrl.GetAssignedRepositories(ctx, wsID, userID)
	if err != nil {
		t.Fatalf("GetAssignedRepositories failed: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("Expected 2 repos, got %d", len(repos))
	}
}

func TestRepositoryAssignment_Concurrency(t *testing.T) {
	store := rbac.NewMemoryRepositoryAssignmentStore()
	ctrl := rbac.NewRepositoryAccessController(store, nil)
	ctx := context.Background()

	wsID := uuid.New()
	var wg sync.WaitGroup
	workers := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			user := uuid.New()
			repo1 := uuid.New()
			repo2 := uuid.New()

			_, _ = ctrl.AssignRepositories(ctx, wsID, user, []uuid.UUID{repo1, repo2}, "admin")
			_ = ctrl.CanAccessRepository(ctx, wsID, user, models.RoleMember, repo1)
			_, _ = ctrl.RevokeRepository(ctx, wsID, user, repo1)
			_, _ = ctrl.GetAssignedRepositories(ctx, wsID, user)
		}(i)
	}

	wg.Wait()
}

type mockDBStore struct {
	saved map[string]*models.UserRepositoryAssignment
}

func newMockDBStore() *mockDBStore {
	return &mockDBStore{saved: make(map[string]*models.UserRepositoryAssignment)}
}

func (m *mockDBStore) key(wsID, uID uuid.UUID) string {
	return wsID.String() + ":" + uID.String()
}

func (m *mockDBStore) SaveRepositoryAssignment(ctx context.Context, a *models.UserRepositoryAssignment) error {
	m.saved[m.key(a.WorkspaceID, a.UserID)] = a
	return nil
}

func (m *mockDBStore) GetRepositoryAssignment(ctx context.Context, wsID, uID uuid.UUID) (*models.UserRepositoryAssignment, error) {
	return m.saved[m.key(wsID, uID)], nil
}

func (m *mockDBStore) DeleteRepositoryAssignment(ctx context.Context, wsID, uID uuid.UUID) error {
	delete(m.saved, m.key(wsID, uID))
	return nil
}

func TestPostgresRepositoryAssignmentStore(t *testing.T) {
	ctx := context.Background()
	mockDB := newMockDBStore()
	pgStore := rbac.NewPostgresRepositoryAssignmentStore(mockDB)
	ctrl := rbac.NewRepositoryAccessController(pgStore, nil)

	wsID := uuid.New()
	userID := uuid.New()
	repo1 := uuid.New()
	repo2 := uuid.New()

	// Assign
	assign, err := ctrl.AssignRepositories(ctx, wsID, userID, []uuid.UUID{repo1, repo2}, "admin@corp.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assign.RepositoryIDs) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(assign.RepositoryIDs))
	}

	// Verify access
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repo1) {
		t.Fatal("expected access to repo1")
	}

	// Revoke
	_, err = ctrl.RevokeRepository(ctx, wsID, userID, repo1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repo1) {
		t.Fatal("expected repo1 to be revoked")
	}
	if !ctrl.CanAccessRepository(ctx, wsID, userID, models.RoleMember, repo2) {
		t.Fatal("expected repo2 to still be granted")
	}

	// Delete
	err = pgStore.DeleteAssignment(ctx, wsID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	remaining, err := pgStore.GetAssignment(ctx, wsID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if remaining != nil {
		t.Fatal("expected assignment to be deleted")
	}
}
