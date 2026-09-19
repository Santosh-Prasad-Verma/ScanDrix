package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/application"
	"github.com/scandrix/backend/internal/identity/domain"
	"github.com/scandrix/backend/internal/identity/infrastructure"
)

func TestCanAccessUseCase(t *testing.T) {
	ctx := context.Background()
	permRepo := infrastructure.NewInMemoryPermissionsRepository()
	factory := application.NewPermissionsAbilityFactory(permRepo)
	uc := application.NewCanAccessUseCase(factory)

	orgID := uuid.New()
	userID := uuid.New()

	// 1. Missing parameter validation
	_, err := uc.Execute(ctx, domain.User{}, domain.ActionRead, domain.ResourcePullRequests, nil)
	if err == nil {
		t.Fatalf("expected error for empty user")
	}

	owner := domain.User{
		UUID:             userID,
		Email:            "owner@scandrix.dev",
		Role:             domain.RoleOwner,
		OrganizationUUID: &orgID,
	}

	// 2. Owner can access org-level resources (Billing, OrgSettings)
	canBilling, err := uc.Execute(ctx, owner, domain.ActionManage, domain.ResourceBilling, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canBilling {
		t.Errorf("expected owner to be able to manage billing")
	}

	// 3. RepoAdmin repo-scoped checks
	adminID := uuid.New()
	repo1 := "repo-100"
	repo2 := "repo-200"
	_, _ = permRepo.Create(ctx, domain.Permissions{
		UUID:                  uuid.New(),
		UserUUID:              adminID,
		AssignedRepositoryIDs: []string{repo1},
	})

	admin := domain.User{
		UUID:             adminID,
		Email:            "admin@scandrix.dev",
		Role:             domain.RoleRepoAdmin,
		OrganizationUUID: &orgID,
	}

	// RepoAdmin cannot manage billing
	canAdminBilling, err := uc.Execute(ctx, admin, domain.ActionManage, domain.ResourceBilling, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canAdminBilling {
		t.Errorf("expected repo admin NOT to be able to manage billing")
	}

	// RepoAdmin can update drixy rules in assigned repo
	canRepo1, err := uc.Execute(ctx, admin, domain.ActionUpdate, domain.ResourceDrixyRules, &repo1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canRepo1 {
		t.Errorf("expected repo admin to update drixy rules in assigned repo1")
	}

	// RepoAdmin cannot update drixy rules in unassigned repo
	canRepo2, err := uc.Execute(ctx, admin, domain.ActionUpdate, domain.ResourceDrixyRules, &repo2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canRepo2 {
		t.Errorf("expected repo admin NOT to update drixy rules in unassigned repo2")
	}
}

func TestGetPermissionsUseCase(t *testing.T) {
	ctx := context.Background()
	permRepo := infrastructure.NewInMemoryPermissionsRepository()
	factory := application.NewPermissionsAbilityFactory(permRepo)
	uc := application.NewGetPermissionsUseCase(factory)

	// Missing org ID should error
	_, err := uc.Execute(ctx, domain.User{UUID: uuid.New()})
	if err == nil {
		t.Fatalf("expected error for missing organization ID")
	}

	orgID := uuid.New()
	user := domain.User{
		UUID:             uuid.New(),
		Role:             domain.RoleRepoAdmin,
		OrganizationUUID: &orgID,
	}

	permMap, err := uc.Execute(ctx, user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if permMap == nil {
		t.Fatalf("expected non-nil permission map")
	}

	// Check that expected resources are in the map
	if _, ok := permMap[domain.ResourcePullRequests]; !ok {
		t.Errorf("expected ResourcePullRequests in permission map")
	}
	if _, ok := permMap[domain.ResourceDrixyRules]; !ok {
		t.Errorf("expected ResourceDrixyRules in permission map")
	}
}

func TestAssignReposUseCase(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	permRepo := infrastructure.NewInMemoryPermissionsRepository()
	auditEmitter := &mockAuditLogEmitter{}
	uc := application.NewAssignReposUseCase(userRepo, permRepo, auditEmitter)

	orgID := uuid.New()
	targetUserID := uuid.New()
	actingUserID := uuid.New()

	// 1. User not found
	_, err := uc.Execute(ctx, application.AssignReposInput{
		ActingUserUUID: actingUserID,
		TargetUserUUID: targetUserID,
		RepoIDs:        []string{"repo-1"},
	})
	if err == nil {
		t.Fatalf("expected error for non-existent user")
	}

	// Create target user
	targetUser := domain.User{
		UUID:             targetUserID,
		Email:            "dev@scandrix.dev",
		Name:             "Dev User",
		Role:             domain.RoleContributor,
		Status:           domain.UserStatusActive,
		OrganizationUUID: &orgID,
	}
	_, _ = userRepo.Create(ctx, targetUser)

	// 2. AvailableRepos whitelist filtering
	// Providing invalid repo IDs should fail if none are valid
	_, err = uc.Execute(ctx, application.AssignReposInput{
		ActingUserUUID: actingUserID,
		TargetUserUUID: targetUserID,
		RepoIDs:        []string{"invalid-repo"},
		AvailableRepos: []string{"org-repo-1", "org-repo-2"},
	})
	if err == nil {
		t.Fatalf("expected error when none of repo IDs match available repos")
	}

	// Providing valid repo IDs
	assigned, err := uc.Execute(ctx, application.AssignReposInput{
		ActingUserUUID: actingUserID,
		ActingEmail:    "admin@scandrix.dev",
		TargetUserUUID: targetUserID,
		RepoIDs:        []string{"org-repo-1", "org-repo-2"},
		AvailableRepos: []string{"org-repo-1", "org-repo-2", "org-repo-3"},
		TeamUUID:       uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error assigning repos: %v", err)
	}
	if len(assigned) != 2 {
		t.Fatalf("expected 2 assigned repos, got: %d", len(assigned))
	}

	// 3. Verify audit log emission
	if len(auditEmitter.events) == 0 {
		t.Fatalf("expected audit log event to be emitted")
	}
	if auditEmitter.events[0] != "USER_REPO_ACCESS" {
		t.Errorf("expected event USER_REPO_ACCESS, got: %s", auditEmitter.events[0])
	}
	addedRepos, ok := auditEmitter.params[0]["added_repositories"].([]string)
	if !ok || len(addedRepos) != 2 {
		t.Errorf("expected 2 added repos in audit payload: %v", auditEmitter.params[0])
	}

	// 4. Update assignment: remove one repo, add another
	auditEmitter.events = nil
	auditEmitter.params = nil
	assigned2, err := uc.Execute(ctx, application.AssignReposInput{
		ActingUserUUID: actingUserID,
		ActingEmail:    "admin@scandrix.dev",
		TargetUserUUID: targetUserID,
		RepoIDs:        []string{"org-repo-2", "org-repo-3"},
		AvailableRepos: []string{"org-repo-1", "org-repo-2", "org-repo-3"},
	})
	if err != nil {
		t.Fatalf("unexpected error updating repos: %v", err)
	}
	if len(assigned2) != 2 {
		t.Fatalf("expected 2 repos, got: %d", len(assigned2))
	}

	if len(auditEmitter.events) != 1 {
		t.Fatalf("expected 1 audit event on update")
	}
	added := auditEmitter.params[0]["added_repositories"].([]string)
	removed := auditEmitter.params[0]["removed_repos"].([]string)
	if len(added) != 1 || added[0] != "org-repo-3" {
		t.Errorf("expected org-repo-3 added, got: %v", added)
	}
	if len(removed) != 1 || removed[0] != "org-repo-1" {
		t.Errorf("expected org-repo-1 removed, got: %v", removed)
	}
}

func TestGetAssignedReposUseCase(t *testing.T) {
	ctx := context.Background()
	permRepo := infrastructure.NewInMemoryPermissionsRepository()
	uc := application.NewGetAssignedReposUseCase(permRepo)

	// 1. Nil UUID returns empty
	repos, err := uc.Execute(ctx, uuid.Nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected empty slice for uuid.Nil")
	}

	// 2. User with no permissions record returns empty
	unknownUser := uuid.New()
	repos, err = uc.Execute(ctx, unknownUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected empty slice for user without permissions")
	}

	// 3. User with assigned repos
	knownUser := uuid.New()
	_, _ = permRepo.Create(ctx, domain.Permissions{
		UUID:                  uuid.New(),
		UserUUID:              knownUser,
		AssignedRepositoryIDs: []string{"repo-alpha", "repo-beta"},
	})

	repos, err = uc.Execute(ctx, knownUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 || repos[0] != "repo-alpha" || repos[1] != "repo-beta" {
		t.Errorf("unexpected assigned repos: %v", repos)
	}
}
