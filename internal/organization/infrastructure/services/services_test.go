// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
	"github.com/scandrix/backend/internal/organization/infrastructure/services"
)

func TestOrganizationService(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresOrganizationRepository(nil)
	svc := services.NewOrganizationService(repo)

	org, err := svc.CreateOrganizationWithTenant(ctx, "ScanDrix Cloud", "")
	if err != nil {
		t.Fatalf("CreateOrganizationWithTenant failed: %v", err)
	}
	if !strings.HasPrefix(org.TenantName, "ScanDrix Cloud-") {
		t.Errorf("expected tenant slug prefix, got %s", org.TenantName)
	}

	track, err := svc.GetReleaseTrack(ctx, org.UUID)
	if err != nil || track != orgdomain.DefaultReleaseTrack {
		t.Errorf("expected default release track (%s), got %s", orgdomain.DefaultReleaseTrack, track)
	}
}

func TestOrganizationParametersService(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresOrganizationParametersRepository(nil)
	svc := services.NewOrganizationParametersService(repo)
	wsID := uuid.New()

	byok := orgparamdomain.BYOKConfigValue{
		Version: 2,
		Credentials: []orgparamdomain.BYOKCredential{
			{ID: "c1", Provider: "openai", APIKey: "sk-enc123"},
			{ID: "c2", Provider: "anthropic", APIKey: "claude-enc123"},
		},
		Models: []orgparamdomain.BYOKModel{
			{ID: "m1", ModelID: "gpt-4o", Provider: "openai"},
		},
	}

	_, err := svc.CreateOrUpdateConfig(ctx, wsID, orgparamdomain.KeyBYOKConfig, byok, "BYOK Config")
	if err != nil {
		t.Fatalf("CreateOrUpdateConfig failed: %v", err)
	}

	// Delete provider credential
	err = svc.DeleteBYOKConfig(ctx, wsID, "openai")
	if err != nil {
		t.Fatalf("DeleteBYOKConfig failed: %v", err)
	}

	// Delete model
	err = svc.DeleteBYOKModel(ctx, wsID, "m1")
	if err != nil {
		t.Fatalf("DeleteBYOKModel failed: %v", err)
	}
}

func TestParametersServiceCached(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresParametersRepository(nil)
	svc := services.NewParametersService(repo)
	wsID := uuid.New()

	reviewCfg := paramdomain.CodeReviewConfigValue{
		MaxFiles:          100,
		SeverityThreshold: "HIGH",
	}
	_, err := svc.CreateOrUpdateConfig(ctx, wsID, nil, paramdomain.KeyCodeReviewConfig, reviewCfg, "Review config")
	if err != nil {
		t.Fatalf("CreateOrUpdateConfig failed: %v", err)
	}

	// Read from cache
	cached, err := svc.FindByKeyCached(ctx, wsID, nil, paramdomain.KeyCodeReviewConfig)
	if err != nil || cached == nil {
		t.Fatalf("FindByKeyCached failed: %v", err)
	}
}

func TestTeamAndMemberServices(t *testing.T) {
	ctx := context.Background()
	teamRepo := repositories.NewPostgresTeamRepository(nil)
	teamSvc := services.NewTeamService(teamRepo)

	memberRepo := repositories.NewPostgresTeamMemberRepository(nil)
	memberSvc := services.NewTeamMembersService(memberRepo)

	wsID := uuid.New()
	team, err := teamSvc.CreateTeam(ctx, wsID, "DevOps", "Infrastructure team")
	if err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}

	invites := []memberdomain.MemberInvitation{
		{Email: "alice@acme.com", Role: memberdomain.RoleAdmin},
		{Email: "bob@acme.com", Role: memberdomain.RoleMember},
	}
	results, err := memberSvc.InviteMembers(ctx, wsID, team.UUID, invites, "admin@acme.com")
	if err != nil {
		t.Fatalf("InviteMembers failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 invite results, got %d", len(results))
	}

	// Re-invite same user -> should report already_member
	results2, _ := memberSvc.InviteMembers(ctx, wsID, team.UUID, invites[:1], "admin@acme.com")
	if len(results2) != 1 || results2[0].Status != "already_member" {
		t.Fatalf("expected already_member status, got %v", results2)
	}
}

func TestTeamCliKeyServiceValidation(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresTeamCliKeyRepository(nil)
	svc := services.NewTeamCliKeyService(repo)
	wsID := uuid.New()

	rawKey, entity, err := svc.GenerateKey(ctx, wsID, nil, "Prod Runner", []byte(`{"allow_dry_run":true}`), nil)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if !strings.HasPrefix(rawKey, "scandrix_") {
		t.Fatalf("expected scandrix_ prefix, got %s", rawKey)
	}

	// Validate Key
	res, err := svc.ValidateKey(ctx, rawKey)
	if err != nil || res == nil {
		t.Fatalf("ValidateKey failed: %v", err)
	}
	if res.KeyID != entity.UUID {
		t.Errorf("expected KeyID %s, got %s", entity.UUID, res.KeyID)
	}

	// Revoked key validation fails
	_ = svc.Revoke(ctx, wsID, entity.UUID)
	_, errRevoked := svc.ValidateKey(ctx, rawKey)
	if errRevoked == nil {
		t.Fatalf("expected error validating revoked key")
	}
}

func TestCliDeviceServiceQuotas(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresCliDeviceRepository(nil)
	svc := services.NewCliDeviceService(repo)
	wsID := uuid.New()

	// Register device 1
	res1, err := svc.ValidateOrRegisterDevice(ctx, wsID, "machine-alpha", "", "scandrix-cli/v1.0.0")
	if err != nil || res1.DeviceToken == "" {
		t.Fatalf("expected device token on first registration: %v", err)
	}

	// Heartbeat with valid token -> returns empty DeviceToken (no refresh needed)
	res2, err := svc.ValidateOrRegisterDevice(ctx, wsID, "machine-alpha", res1.DeviceToken, "scandrix-cli/v1.0.0")
	if err != nil {
		t.Fatalf("ValidateOrRegisterDevice failed: %v", err)
	}
	if res2.DeviceToken != "" {
		t.Fatalf("expected empty token for validated device, got %s", res2.DeviceToken)
	}
}

func TestGlobalParametersService(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresGlobalParametersRepository(nil)
	svc := services.NewGlobalParametersService(repo)

	param, err := svc.CreateOrUpdateConfig(ctx, "max_review_files", 500, "Maximum review files allowed globally")
	if err != nil || param == nil {
		t.Fatalf("CreateOrUpdateConfig failed: %v", err)
	}

	found, err := svc.FindByKey(ctx, "max_review_files")
	if err != nil || found == nil {
		t.Fatalf("FindByKey failed: %v", err)
	}
}
