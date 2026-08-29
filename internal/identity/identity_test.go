package identity_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity"
)

func TestPermissionsEngineMatrix(t *testing.T) {
	engine := identity.NewPermissionsEngine()
	wsID := uuid.New()

	adminUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleAdmin,
	}

	maintainerUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleMaintainer,
	}

	reviewerUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleReviewer,
	}

	viewerUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleViewer,
	}

	// 1. Admin checks
	if !engine.Can(adminUser, identity.ActionDelete, identity.ResourceWorkspace) {
		t.Fatal("admin should be allowed to delete workspace")
	}
	if !engine.Can(adminUser, identity.ActionUpdate, identity.ResourceBYOK) {
		t.Fatal("admin should be allowed to update BYOK")
	}

	// 2. Maintainer checks
	if engine.Can(maintainerUser, identity.ActionDelete, identity.ResourceWorkspace) {
		t.Fatal("maintainer should NOT be allowed to delete workspace")
	}
	if engine.Can(maintainerUser, identity.ActionUpdate, identity.ResourceBYOK) {
		t.Fatal("maintainer should NOT be allowed to update BYOK")
	}
	if !engine.Can(maintainerUser, identity.ActionTrigger, identity.ResourceReview) {
		t.Fatal("maintainer should be allowed to trigger review")
	}

	// 3. Reviewer checks
	if !engine.Can(reviewerUser, identity.ActionTrigger, identity.ResourceReview) {
		t.Fatal("reviewer should be allowed to trigger review")
	}
	if engine.Can(reviewerUser, identity.ActionCreate, identity.ResourceRepo) {
		t.Fatal("reviewer should NOT be allowed to create repositories")
	}

	// 4. Viewer checks
	if engine.Can(viewerUser, identity.ActionTrigger, identity.ResourceReview) {
		t.Fatal("viewer should NOT be allowed to trigger reviews")
	}
	if !engine.Can(viewerUser, identity.ActionRead, identity.ResourceReview) {
		t.Fatal("viewer should be allowed to read reviews")
	}
}

func TestProfileService(t *testing.T) {
	svc := identity.NewProfileService()
	uid := uuid.New()
	wsID := uuid.New()

	profile := identity.UserProfile{
		ID:          uid,
		WorkspaceID: wsID,
		Email:       "engineer@scandrix.io",
		DisplayName: "Lead Security Engineer",
		Role:        identity.RoleMaintainer,
	}

	// 1. Create Profile
	err := svc.CreateProfile(profile)
	if err != nil {
		t.Fatalf("failed creating profile: %v", err)
	}

	// 2. Prevent duplicate creation
	errDup := svc.CreateProfile(profile)
	if errDup == nil {
		t.Fatal("expected error creating duplicate profile ID")
	}

	// 3. Update Preferences
	err = svc.UpdatePreference(uid, "cli_theme", "dark-high-contrast")
	if err != nil {
		t.Fatalf("failed updating preference: %v", err)
	}

	// 4. Get Profile
	retrieved, err := svc.GetProfile(uid)
	if err != nil {
		t.Fatalf("failed retrieving profile: %v", err)
	}
	if retrieved.Preferences["cli_theme"] != "dark-high-contrast" {
		t.Fatalf("unexpected preference: %v", retrieved.Preferences)
	}
}
