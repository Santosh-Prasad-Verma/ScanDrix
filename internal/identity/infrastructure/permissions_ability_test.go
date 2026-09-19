package infrastructure

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

func TestRolePolicies(t *testing.T) {
	// 1. RoleUsesRepoAssignment tests
	if !RoleUsesRepoAssignment(domain.RoleRepoAdmin) {
		t.Errorf("expected RepoAdmin to use repo assignment")
	}
	if RoleUsesRepoAssignment(domain.RoleOwner) {
		t.Errorf("expected Owner to NOT use repo assignment")
	}
	if RoleUsesRepoAssignment(domain.RoleContributor) {
		t.Errorf("expected Contributor to NOT use repo assignment")
	}
	if RoleUsesRepoAssignment(domain.RoleBillingManager) {
		t.Errorf("expected BillingManager to NOT use repo assignment")
	}

	// 2. Owner permissions
	factory := NewAbilityFactory()
	ownerUser := domain.User{UUID: uuid.New(), Name: "Owner"}
	ownerAbility := factory.CreateForUser(ownerUser, domain.RoleOwner, nil)

	if !ownerAbility.Can("manage", "all") {
		t.Errorf("expected owner to have manage all")
	}
	if !ownerAbility.Can("update", "drixy_rules") {
		t.Errorf("expected owner to be able to update drixy_rules")
	}

	// 3. Contributor permissions (read-only org wide, cannot update)
	contribUser := domain.User{UUID: uuid.New(), Name: "Contributor"}
	contribAbility := factory.CreateForUser(contribUser, domain.RoleContributor, nil)

	if !contribAbility.Can("read", "drixy_rules") {
		t.Errorf("expected contributor to read drixy rules")
	}
	if contribAbility.Can("update", "drixy_rules") {
		t.Errorf("contributor must NOT update drixy rules")
	}
	if contribAbility.Can("create", "drixy_rules") {
		t.Errorf("contributor must NOT create drixy rules")
	}

	// 4. RepoAdmin with assigned repos
	repoAdminUser := domain.User{UUID: uuid.New(), Name: "RepoAdmin"}
	adminAbility := factory.CreateForUser(repoAdminUser, domain.RoleRepoAdmin, []string{"repo-1", "repo-2"})

	// Read is org-wide
	if !adminAbility.Can("read", "drixy_rules") {
		t.Errorf("expected repo admin to read drixy rules")
	}
	// Write is repo-scoped
	if !adminAbility.CanAccessRepo("repo-1") {
		t.Errorf("expected access to assigned repo-1")
	}
	if adminAbility.CanAccessRepo("repo-3") {
		t.Errorf("repo admin must not access unassigned repo-3")
	}
}

func TestPolicyGuard(t *testing.T) {
	ctx := context.Background()
	factory := NewAbilityFactory()
	guard := NewPolicyGuard(factory)

	user := domain.User{UUID: uuid.New(), Name: "Admin"}
	handler := &GenericPolicyHandler{
		Action:  domain.ActionUpdate,
		Subject: domain.ResourceDrixyRules,
	}

	// RepoAdmin on assigned repo-1 -> Allowed
	allowed := guard.CanActivate(ctx, user, domain.RoleRepoAdmin, []string{"repo-1"}, []PolicyHandler{handler}, "repo-1")
	if !allowed {
		t.Errorf("expected allowed for assigned repo-1")
	}

	// Contributor on repo-1 -> Denied
	denied := guard.CanActivate(ctx, user, domain.RoleContributor, []string{"repo-1"}, []PolicyHandler{handler}, "repo-1")
	if denied {
		t.Errorf("expected denied for contributor attempting write")
	}
}
