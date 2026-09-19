package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/application"
	"github.com/scandrix/backend/internal/identity/domain"
	"github.com/scandrix/backend/internal/identity/infrastructure"
)

func TestCreateProfileUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryProfileRepository()
	uc := application.NewCreateProfileUseCase(repo)

	// 1. Missing user UUID
	err := uc.Execute(ctx, domain.UserProfile{})
	if err == nil {
		t.Fatalf("expected error when UserUUID is nil")
	}

	// 2. Valid profile creation
	userUUID := uuid.New()
	profile := domain.UserProfile{
		UUID:      uuid.New(),
		UserUUID:  userUUID,
		Name:      "Test Developer",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	err = uc.Execute(ctx, profile)
	if err != nil {
		t.Fatalf("unexpected error creating profile: %v", err)
	}

	found, err := repo.FindByUserUUID(ctx, userUUID)
	if err != nil || found == nil {
		t.Fatalf("expected profile to be found in repository")
	}
	if found.UserUUID != userUUID {
		t.Errorf("expected user UUID %s, got %s", userUUID, found.UserUUID)
	}
}

func TestUpdateProfileUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryProfileRepository()
	uc := application.NewUpdateProfileUseCase(repo)

	// 1. Nil UUID validation
	_, err := uc.Execute(ctx, uuid.Nil, map[string]any{"phone": "+1234567890"})
	if err == nil {
		t.Fatalf("expected error for nil profile UUID")
	}

	// 2. Non-existent profile
	_, err = uc.Execute(ctx, uuid.New(), map[string]any{"phone": "+1234567890"})
	if err == nil {
		t.Fatalf("expected error for non-existent profile")
	}

	// 3. Valid update
	profUUID := uuid.New()
	userUUID := uuid.New()
	_, _ = repo.Create(ctx, domain.UserProfile{
		UUID:      profUUID,
		UserUUID:  userUUID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})

	updated, err := uc.Execute(ctx, profUUID, map[string]any{
		"phone":    "+15550001",
		"position": "Lead Engineer",
	})
	if err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if updated.Phone != "+15550001" {
		t.Errorf("expected phone to be updated, got: %s", updated.Phone)
	}
	if updated.Position != "Lead Engineer" {
		t.Errorf("expected position to be updated, got: %s", updated.Position)
	}
}

func TestSaveMarketingSurveyUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryProfileRepository()
	uc := application.NewSaveMarketingSurveyUseCase(repo)

	userUUID := uuid.New()

	// 1. Profile not found
	ref := "github"
	goal := "automated_code_reviews"
	err := uc.Execute(ctx, userUUID, &ref, &goal)
	if err == nil {
		t.Fatalf("expected error when profile does not exist")
	}

	// 2. Create profile and save survey
	profUUID := uuid.New()
	_, _ = repo.Create(ctx, domain.UserProfile{
		UUID:      profUUID,
		UserUUID:  userUUID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})

	err = uc.Execute(ctx, userUUID, &ref, &goal)
	if err != nil {
		t.Fatalf("unexpected error saving survey: %v", err)
	}

	found, _ := repo.FindByUserUUID(ctx, userUUID)
	if found.ReferralSource != "github" {
		t.Errorf("expected referral_source 'github', got: %s", found.ReferralSource)
	}
	if found.PrimaryGoal != "automated_code_reviews" {
		t.Errorf("expected primary_goal 'automated_code_reviews', got: %s", found.PrimaryGoal)
	}

	// 3. No updates provided is a no-op
	err = uc.Execute(ctx, userUUID, nil, nil)
	if err != nil {
		t.Fatalf("expected nil error for no-op survey: %v", err)
	}
}

func TestProfileConfigService(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryProfileConfigRepository()
	service := application.NewProfileConfigService(repo)

	profUUID := uuid.New()
	key := domain.ProfileConfigUserNotifications

	// 1. Get non-existent config
	cfg, err := service.GetConfig(ctx, profUUID, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config for unset key")
	}

	// 2. Set new config
	created, err := service.SetConfig(ctx, profUUID, key, true)
	if err != nil {
		t.Fatalf("unexpected error setting config: %v", err)
	}
	if created == nil || created.ConfigValue != true {
		t.Errorf("expected config value true, got: %v", created)
	}

	// 3. Get existing config
	fetched, err := service.GetConfig(ctx, profUUID, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fetched == nil || fetched.ConfigValue != true {
		t.Errorf("expected fetched config value true")
	}

	// 4. Update existing config
	updated, err := service.SetConfig(ctx, profUUID, key, false)
	if err != nil {
		t.Fatalf("unexpected error updating config: %v", err)
	}
	if updated.ConfigValue != false {
		t.Errorf("expected updated config value false, got: %v", updated.ConfigValue)
	}

	// 5. List configs
	list, err := service.ListConfigs(ctx, profUUID)
	if err != nil {
		t.Fatalf("unexpected error listing configs: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 config, got: %d", len(list))
	}
}
