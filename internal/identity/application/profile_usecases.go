package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// CreateProfileUseCase ensures a user profile exists.
type CreateProfileUseCase struct {
	profileRepo domain.ProfileRepository
}

func NewCreateProfileUseCase(repo domain.ProfileRepository) *CreateProfileUseCase {
	return &CreateProfileUseCase{profileRepo: repo}
}

func (uc *CreateProfileUseCase) Execute(ctx context.Context, profile domain.UserProfile) error {
	if profile.UserUUID == uuid.Nil {
		return errors.New("user UUID is required to create profile")
	}

	return uc.profileRepo.UpdateByUserID(ctx, profile.UserUUID, profile)
}

// UpdateProfileUseCase updates profile attributes.
type UpdateProfileUseCase struct {
	profileRepo domain.ProfileRepository
}

func NewUpdateProfileUseCase(repo domain.ProfileRepository) *UpdateProfileUseCase {
	return &UpdateProfileUseCase{profileRepo: repo}
}

func (uc *UpdateProfileUseCase) Execute(ctx context.Context, profileUUID uuid.UUID, updates map[string]any) (*domain.UserProfile, error) {
	if profileUUID == uuid.Nil {
		return nil, errors.New("invalid profile UUID")
	}

	return uc.profileRepo.Update(ctx, profileUUID, updates)
}

// SaveMarketingSurveyUseCase records user onboarding survey responses.
type SaveMarketingSurveyUseCase struct {
	profileRepo domain.ProfileRepository
}

func NewSaveMarketingSurveyUseCase(repo domain.ProfileRepository) *SaveMarketingSurveyUseCase {
	return &SaveMarketingSurveyUseCase{profileRepo: repo}
}

func (uc *SaveMarketingSurveyUseCase) Execute(ctx context.Context, userUUID uuid.UUID, referralSource, primaryGoal *string) error {
	profile, err := uc.profileRepo.FindByUserUUID(ctx, userUUID)
	if err != nil || profile == nil {
		return errors.New("profile not found")
	}

	updates := make(map[string]any)
	if referralSource != nil {
		updates["referral_source"] = *referralSource
	}
	if primaryGoal != nil {
		updates["primary_goal"] = *primaryGoal
	}

	if len(updates) > 0 {
		_, err = uc.profileRepo.Update(ctx, profile.UUID, updates)
		return err
	}
	return nil
}

// ProfileConfigService manages user preferences such as notification settings.
type ProfileConfigService struct {
	configRepo domain.ProfileConfigRepository
}

func NewProfileConfigService(repo domain.ProfileConfigRepository) *ProfileConfigService {
	return &ProfileConfigService{configRepo: repo}
}

func (s *ProfileConfigService) GetConfig(
	ctx context.Context,
	profileUUID uuid.UUID,
	key domain.ProfileConfigKey,
) (*domain.ProfileConfig, error) {
	return s.configRepo.FindOne(ctx, profileUUID, key)
}

func (s *ProfileConfigService) SetConfig(
	ctx context.Context,
	profileUUID uuid.UUID,
	key domain.ProfileConfigKey,
	value any,
) (*domain.ProfileConfig, error) {
	existing, _ := s.configRepo.FindOne(ctx, profileUUID, key)
	if existing != nil {
		return s.configRepo.Update(ctx, existing.UUID, value, true)
	}

	now := time.Now().UTC()
	newConfig := domain.ProfileConfig{
		UUID:        uuid.New(),
		ProfileUUID: profileUUID,
		ConfigKey:   key,
		ConfigValue: value,
		Status:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return s.configRepo.Create(ctx, newConfig)
}

func (s *ProfileConfigService) ListConfigs(
	ctx context.Context,
	profileUUID uuid.UUID,
) ([]domain.ProfileConfig, error) {
	return s.configRepo.Find(ctx, profileUUID)
}
