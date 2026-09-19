// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package usecases_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	onboardingusecases "github.com/scandrix/backend/internal/organization/application/usecases/onboarding"
	orgusecases "github.com/scandrix/backend/internal/organization/application/usecases/organization"
	orgparamusecases "github.com/scandrix/backend/internal/organization/application/usecases/organizationparameters"
	paramusecases "github.com/scandrix/backend/internal/organization/application/usecases/parameters"
	teamusecases "github.com/scandrix/backend/internal/organization/application/usecases/team"
	memberusecases "github.com/scandrix/backend/internal/organization/application/usecases/teammembers"
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
	"github.com/scandrix/backend/pkg/models"
)

type mockRepoReader struct {
	repos []models.TrackedRepository
}

func (m *mockRepoReader) ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error) {
	return m.repos, nil
}

func TestOrganizationUseCases(t *testing.T) {
	ctx := context.Background()
	orgRepo := repositories.NewPostgresOrganizationRepository(nil)
	paramRepo := repositories.NewPostgresOrganizationParametersRepository(nil)

	org := orgdomain.NewOrganizationEntity("ScanDrix Security", "scandrix-security")
	_, err := orgRepo.Create(ctx, org)
	if err != nil {
		t.Fatalf("failed creating org: %v", err)
	}

	// 1. GetOrganizationNameUseCase
	getNameUC := orgusecases.NewGetOrganizationNameUseCase(orgRepo)
	name, err := getNameUC.Execute(ctx, org.UUID)
	if err != nil || name != "ScanDrix Security" {
		t.Errorf("expected ScanDrix Security, got %s", name)
	}

	// 2. GetReleaseTrackUseCase
	getTrackUC := orgusecases.NewGetReleaseTrackUseCase(orgRepo)
	track, err := getTrackUC.Execute(ctx, org.UUID)
	if err != nil || track != orgdomain.DefaultReleaseTrack {
		t.Errorf("expected default release track (%s), got %s", orgdomain.DefaultReleaseTrack, track)
	}

	// 3. UpdateInfosUseCase
	updateUC := orgusecases.NewUpdateInfosUseCase(orgRepo)
	err = updateUC.Execute(ctx, org.UUID, "ScanDrix Enterprise")
	if err != nil {
		t.Fatalf("UpdateInfosUseCase failed: %v", err)
	}

	// 4. GetOrganizationLanguageUseCase
	reader := &mockRepoReader{
		repos: []models.TrackedRepository{
			{NamespacePath: "backend-go-service"},
			{NamespacePath: "cli-go-tool"},
			{NamespacePath: "frontend-web"},
		},
	}
	getLangUC := orgusecases.NewGetOrganizationLanguageUseCase(reader)
	lang, err := getLangUC.Execute(ctx, org.UUID, nil, "", 5)
	if err != nil || lang != "go" {
		t.Errorf("expected go dominant language, got %s", lang)
	}

	// 5. GetOrganizationsByDomainUseCase
	autoJoin := orgparamdomain.AutoJoinConfigValue{
		Enabled: true,
		Domains: []string{"scandrix.io"},
	}
	paramEntity, _ := orgparamdomain.NewOrganizationParametersEntity(org.UUID, orgparamdomain.KeyAutoJoinConfig, autoJoin, "Auto join")
	_, _ = paramRepo.Create(ctx, paramEntity)

	getDomainUC := orgusecases.NewGetOrganizationsByDomainUseCase(orgRepo, paramRepo)
	matched, err := getDomainUC.Execute(ctx, "scandrix.io")
	if err != nil || len(matched) != 1 {
		t.Fatalf("expected 1 matched org for scandrix.io, got %d", len(matched))
	}
}

func TestTeamAndMemberUseCases(t *testing.T) {
	ctx := context.Background()
	teamRepo := repositories.NewPostgresTeamRepository(nil)
	memberRepo := repositories.NewPostgresTeamMemberRepository(nil)
	paramRepo := repositories.NewPostgresParametersRepository(nil)
	wsID := uuid.New()

	// 1. CreateTeamUseCase
	createTeamUC := teamusecases.NewCreateTeamUseCase(teamRepo, paramRepo)
	team, err := createTeamUC.Execute(ctx, wsID, "Platform SecOps", "Security team", "round_robin")
	if err != nil {
		t.Fatalf("CreateTeamUseCase failed: %v", err)
	}

	// 2. ListTeamsUseCase
	listTeamsUC := teamusecases.NewListTeamsUseCase(teamRepo)
	teams, err := listTeamsUC.Execute(ctx, wsID)
	if err != nil || len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}

	// 3. CreateOrUpdateTeamMembersUseCase
	createMemberUC := memberusecases.NewCreateOrUpdateTeamMembersUseCase(memberRepo)
	invites := []memberdomain.MemberItem{
		{Email: "bob@acme.com", TeamRole: memberdomain.RoleMember, Active: true},
		{Email: "alice@acme.com", TeamRole: memberdomain.RoleAdmin, Active: true},
	}
	results, err := createMemberUC.Execute(ctx, wsID, team.UUID, invites, "lead@acme.com")
	if err != nil || len(results.Results) != 2 {
		t.Fatalf("CreateOrUpdateTeamMembersUseCase failed: %v", err)
	}

	// 4. GetTeamMembersUseCase (sorted alphabetically by email)
	getMembersUC := memberusecases.NewGetTeamMembersUseCase(memberRepo)
	members, err := getMembersUC.Execute(ctx, wsID, team.UUID, true)
	if err != nil || len(members) != 2 {
		t.Fatalf("GetTeamMembersUseCase failed: %v", err)
	}
	if members[0].Email != "alice@acme.com" || members[1].Email != "bob@acme.com" {
		t.Errorf("expected sorted members [alice, bob], got [%s, %s]", members[0].Email, members[1].Email)
	}

	// 5. DeleteTeamMemberUseCase
	deleteMemberUC := memberusecases.NewDeleteTeamMemberUseCase(memberRepo)
	_, err = deleteMemberUC.Execute(ctx, wsID, members[0].UUID, &members[1].UserID, false)
	if err != nil {
		t.Fatalf("DeleteTeamMemberUseCase failed: %v", err)
	}
}

func TestParametersUseCases(t *testing.T) {
	ctx := context.Background()
	paramRepo := repositories.NewPostgresParametersRepository(nil)
	wsID := uuid.New()

	// Default config
	defUC := paramusecases.NewGetDefaultConfigUseCase()
	defCfg := defUC.DefaultCodeReviewConfig()
	if defCfg.MaxFiles <= 0 {
		t.Errorf("expected positive default maxFiles, got %d", defCfg.MaxFiles)
	}

	// Create or update
	createUC := paramusecases.NewCreateOrUpdateParametersUseCase(paramRepo)
	created, err := createUC.Execute(ctx, wsID, nil, paramdomain.KeyCodeReviewConfig, defCfg, "Default review parameters")
	if err != nil || created == nil {
		t.Fatalf("CreateOrUpdateParametersUseCase failed: %v", err)
	}

	// Find by key
	findUC := paramusecases.NewFindByKeyParametersUseCase(paramRepo)
	found, err := findUC.Execute(ctx, wsID, nil, paramdomain.KeyCodeReviewConfig)
	if err != nil || found == nil {
		t.Fatalf("FindByKeyUseCase failed: %v", err)
	}
}

func TestOrganizationParametersUseCases(t *testing.T) {
	ctx := context.Background()
	paramRepo := repositories.NewPostgresOrganizationParametersRepository(nil)
	wsID := uuid.New()

	// Create or update organization parameters
	createOrgParamUC := orgparamusecases.NewCreateOrUpdateUseCase(paramRepo)
	overrides := []orgparamusecases.ModelOverride{
		{ID: "ov-1", WorkspaceID: wsID, Scope: orgparamusecases.ScopeGlobal, ModelID: "gpt-4o-mini", Reason: "Security review"},
	}
	_, err := createOrgParamUC.Execute(ctx, wsID, orgparamdomain.KeyModelOverrides, overrides, "Model overrides")
	if err != nil {
		t.Fatalf("CreateOrUpdateUseCase failed: %v", err)
	}

	listModelUC := orgparamusecases.NewListModelOverridesUseCase(paramRepo)
	listedOverrides, err := listModelUC.Execute(ctx, wsID)
	if err != nil || len(listedOverrides) != 1 {
		t.Fatalf("ListModelOverridesUseCase failed: %v", err)
	}

	clearModelUC := orgparamusecases.NewClearModelOverridesUseCase(paramRepo)
	err = clearModelUC.Execute(ctx, wsID, "")
	if err != nil {
		t.Fatalf("ClearModelOverridesUseCase failed: %v", err)
	}

	// Cockpit Metrics Visibility
	visUC := orgparamusecases.NewGetCockpitMetricsVisibilityUseCase(paramRepo)
	vis, err := visUC.Execute(ctx, wsID)
	if err != nil || !vis.OverallDORAMetrics {
		t.Fatalf("GetCockpitMetricsVisibilityUseCase expected default true metrics, got: %v", vis)
	}
}

// Mock User Account Repository for Onboarding
type mockUserAccountRepo struct {
	users map[uuid.UUID]*onboardingusecases.UserAccount
}

func (m *mockUserAccountRepo) FindByID(ctx context.Context, id uuid.UUID) (*onboardingusecases.UserAccount, error) {
	return m.users[id], nil
}

func (m *mockUserAccountRepo) FindByWorkspaceID(ctx context.Context, wsID uuid.UUID) ([]*onboardingusecases.UserAccount, error) {
	var list []*onboardingusecases.UserAccount
	for _, u := range m.users {
		if u.WorkspaceID == wsID {
			list = append(list, u)
		}
	}
	return list, nil
}

func (m *mockUserAccountRepo) UpdateWorkspaceAndRole(ctx context.Context, userID, newWsID uuid.UUID, role, status string) (*onboardingusecases.UserAccount, error) {
	u := m.users[userID]
	if u != nil {
		u.WorkspaceID = newWsID
		u.Role = role
		u.Status = status
	}
	return u, nil
}

func (m *mockUserAccountRepo) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	delete(m.users, userID)
	return nil
}

func TestJoinOrganizationUseCase(t *testing.T) {
	ctx := context.Background()
	userRepo := &mockUserAccountRepo{users: make(map[uuid.UUID]*onboardingusecases.UserAccount)}
	orgRepo := repositories.NewPostgresOrganizationRepository(nil)
	teamRepo := repositories.NewPostgresTeamRepository(nil)
	memberRepo := repositories.NewPostgresTeamMemberRepository(nil)
	paramRepo := repositories.NewPostgresParametersRepository(nil)

	// Create user with orphaned personal workspace
	oldWsID := uuid.New()
	userID := uuid.New()
	user := &onboardingusecases.UserAccount{
		ID:          userID,
		WorkspaceID: oldWsID,
		Email:       "newdev@enterprise.com",
		DisplayName: "New Developer",
		Role:        "OWNER",
		Status:      "ACTIVE",
	}
	userRepo.users[userID] = user

	oldOrg := orgdomain.NewOrganizationEntity("Personal Org", "personal-org")
	oldOrg.UUID = oldWsID
	_, _ = orgRepo.Create(ctx, oldOrg)

	oldTeam := teamdomain.NewTeamEntity(oldWsID, "Personal Team", "", "round_robin")
	_, _ = teamRepo.Create(ctx, oldTeam)

	// Create target organization and team
	targetWsID := uuid.New()
	targetOrg := orgdomain.NewOrganizationEntity("Enterprise Corp", "enterprise-corp")
	targetOrg.UUID = targetWsID
	_, _ = orgRepo.Create(ctx, targetOrg)

	targetTeam := teamdomain.NewTeamEntity(targetWsID, "Engineering Squad", "", "round_robin")
	_, _ = teamRepo.Create(ctx, targetTeam)

	joinUC := onboardingusecases.NewJoinOrganizationUseCase(userRepo, orgRepo, teamRepo, memberRepo, paramRepo)
	updatedUser, err := joinUC.Execute(ctx, onboardingusecases.JoinOrganizationInput{
		UserID:         userID,
		OrganizationID: targetWsID,
	})
	if err != nil {
		t.Fatalf("JoinOrganizationUseCase failed: %v", err)
	}

	if updatedUser.WorkspaceID != targetWsID {
		t.Errorf("expected user WorkspaceID %s, got %s", targetWsID, updatedUser.WorkspaceID)
	}

	// Verify old personal workspace was cleaned up
	oldOrgCheck, _ := orgRepo.FindByID(ctx, oldWsID)
	if oldOrgCheck != nil {
		t.Errorf("expected orphaned personal workspace to be purged")
	}
}
