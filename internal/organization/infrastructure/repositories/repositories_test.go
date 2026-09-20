// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	clidevicedomain "github.com/scandrix/backend/internal/organization/domain/clidevice"
	globalparamdomain "github.com/scandrix/backend/internal/organization/domain/globalparameters"
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
	clikey "github.com/scandrix/backend/internal/organization/domain/teamclikey"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
)

func TestOrganizationRepository(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresOrganizationRepository(nil)

	org := orgdomain.NewOrganizationEntity("Acme Corp", "acme-corp")
	created, err := repo.Create(ctx, org)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.UUID == uuid.Nil {
		t.Fatalf("expected non-nil UUID")
	}

	found, err := repo.FindByID(ctx, created.UUID)
	if err != nil || found == nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found.Name != "Acme Corp" {
		t.Errorf("expected Acme Corp, got %s", found.Name)
	}

	// Update
	updateData := *found
	updateData.Name = "Acme Corp International"
	updated, err := repo.Update(ctx, orgdomain.OrganizationFilter{UUID: &created.UUID}, &updateData)
	if err != nil || updated.Name != "Acme Corp International" {
		t.Fatalf("Update failed: %v", err)
	}

	// Delete
	err = repo.Delete(ctx, created.UUID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	afterDel, _ := repo.FindByID(ctx, created.UUID)
	if afterDel != nil {
		t.Fatalf("expected nil after delete")
	}
}

func TestOrganizationParametersRepository(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresOrganizationParametersRepository(nil)
	wsID := uuid.New()

	autoJoin := orgparamdomain.AutoJoinConfigValue{
		Enabled: true,
		Domains: []string{"acme.com", "partner.com"},
	}
	entity, err := orgparamdomain.NewOrganizationParametersEntity(wsID, orgparamdomain.KeyAutoJoinConfig, autoJoin, "Auto join configuration")
	if err != nil {
		t.Fatalf("failed creating entity: %v", err)
	}

	created, err := repo.Create(ctx, entity)
	if err != nil || created == nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := repo.FindByKey(ctx, wsID, orgparamdomain.KeyAutoJoinConfig)
	if err != nil || found == nil {
		t.Fatalf("FindByKey failed: %v", err)
	}
	if found.ConfigKey != orgparamdomain.KeyAutoJoinConfig {
		t.Errorf("expected KeyAutoJoinConfig, got %s", found.ConfigKey)
	}

	// Delete
	err = repo.Delete(ctx, wsID, orgparamdomain.KeyAutoJoinConfig)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestParametersRepository(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresParametersRepository(nil)
	wsID := uuid.New()
	teamID := uuid.New()

	reviewConfig := paramdomain.CodeReviewConfigValue{
		MaxFiles:          50,
		SeverityThreshold: "MEDIUM",
		DrixyRulesEnabled: true,
	}
	entity, err := paramdomain.NewParametersEntity(wsID, &teamID, paramdomain.KeyCodeReviewConfig, reviewConfig, "Team review behavior")
	if err != nil {
		t.Fatalf("failed creating entity: %v", err)
	}

	created, err := repo.Create(ctx, entity)
	if err != nil || created == nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := repo.FindByKey(ctx, wsID, &teamID, paramdomain.KeyCodeReviewConfig)
	if err != nil || found == nil {
		t.Fatalf("FindByKey failed: %v", err)
	}

	err = repo.DeleteByTeamID(ctx, teamID)
	if err != nil {
		t.Fatalf("DeleteByTeamID failed: %v", err)
	}
}

func TestTeamAndMemberRepositories(t *testing.T) {
	ctx := context.Background()
	teamRepo := repositories.NewPostgresTeamRepository(nil)
	memberRepo := repositories.NewPostgresTeamMemberRepository(nil)
	wsID := uuid.New()

	// 1. Team CRUD
	team := teamdomain.NewTeamEntity(wsID, "Backend Core", "Core platform squad", "round_robin")
	createdTeam, err := teamRepo.Create(ctx, team)
	if err != nil || createdTeam == nil {
		t.Fatalf("Team Create failed: %v", err)
	}

	teams, err := teamRepo.FindByWorkspaceID(ctx, wsID)
	if err != nil || len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}

	// 2. Member CRUD
	userID := uuid.New()
	member := memberdomain.NewTeamMemberEntity(wsID, createdTeam.UUID, userID, "lead@acme.com", "Team Lead", memberdomain.RoleAdmin)
	createdMember, err := memberRepo.Create(ctx, member)
	if err != nil || createdMember == nil {
		t.Fatalf("Member Create failed: %v", err)
	}

	members, err := memberRepo.FindManyByWorkspaceID(ctx, wsID)
	if err != nil || len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}

	err = memberRepo.Delete(ctx, createdTeam.UUID, userID)
	if err != nil {
		t.Fatalf("Member Delete failed: %v", err)
	}
}

func TestTeamCliKeyAndDeviceRepositories(t *testing.T) {
	ctx := context.Background()
	keyRepo := repositories.NewPostgresTeamCliKeyRepository(nil)
	devRepo := repositories.NewPostgresCliDeviceRepository(nil)
	wsID := uuid.New()

	// CLI Key
	keyEntity := &clikey.TeamCliKeyEntity{
		UUID:        uuid.New(),
		WorkspaceID: wsID,
		Name:        "CI Key",
		KeyHash:     "$2a$10$xyz",
		KeyPrefix:   "abcdef12",
		Active:      true,
		CreatedAt:   time.Now().UTC(),
	}
	err := keyRepo.Create(ctx, keyEntity)
	if err != nil {
		t.Fatalf("Key Create failed: %v", err)
	}

	foundKey, err := keyRepo.FindByKeyPrefix(ctx, "abcdef12")
	if err != nil || foundKey == nil {
		t.Fatalf("FindByKeyPrefix failed: %v", err)
	}

	err = keyRepo.Revoke(ctx, wsID, keyEntity.UUID)
	if err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	// Device
	devEntity := &clidevicedomain.CliDeviceEntity{
		UUID:            uuid.New(),
		WorkspaceID:     wsID,
		DeviceID:        "dev-macbook-m3",
		DeviceTokenHash: "hash123",
		UserAgent:       "scandrix-cli/v1.0.0",
		LastSeenAt:      time.Now().UTC(),
	}
	err = devRepo.Create(ctx, devEntity)
	if err != nil {
		t.Fatalf("Device Create failed: %v", err)
	}

	count, err := devRepo.CountByWorkspaceID(ctx, wsID)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 device, got %d", count)
	}
}

func TestGlobalParametersRepository(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresGlobalParametersRepository(nil)

	entity, err := globalparamdomain.NewGlobalParametersEntity("telemetry_heartbeat", map[string]any{"rate_seconds": 60}, "Heartbeat rate")
	if err != nil {
		t.Fatalf("failed creating entity: %v", err)
	}

	err = repo.Create(ctx, entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := repo.FindByKey(ctx, "telemetry_heartbeat")
	if err != nil || found == nil {
		t.Fatalf("FindByKey failed: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 global parameter, got %d", len(list))
	}
}
