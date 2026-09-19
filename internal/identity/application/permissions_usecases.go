package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// CanAccessUseCase checks whether a user has permission to perform an action on a resource.
type CanAccessUseCase struct {
	abilityFactory *PermissionsAbilityFactory
}

func NewCanAccessUseCase(factory *PermissionsAbilityFactory) *CanAccessUseCase {
	return &CanAccessUseCase{abilityFactory: factory}
}

func (uc *CanAccessUseCase) Execute(
	ctx context.Context,
	user domain.User,
	action domain.Action,
	resource domain.ResourceType,
	repoID *string,
) (bool, error) {
	if user.UUID == uuid.Nil || user.OrganizationUUID == nil || action == "" || resource == "" {
		return false, errors.New("missing required parameters in can-access use case")
	}

	ability, err := uc.abilityFactory.CreateForUser(ctx, user, nil)
	if err != nil {
		return false, fmt.Errorf("error creating ability for user: %w", err)
	}

	if repoID != nil && *repoID != "" {
		return ability.CanInRepo(action, resource, *repoID), nil
	}
	return ability.Can(action, resource), nil
}

// GetPermissionsUseCase retrieves the serialized permission matrix for the frontend dashboard.
type GetPermissionsUseCase struct {
	abilityFactory *PermissionsAbilityFactory
}

func NewGetPermissionsUseCase(factory *PermissionsAbilityFactory) *GetPermissionsUseCase {
	return &GetPermissionsUseCase{abilityFactory: factory}
}

func (uc *GetPermissionsUseCase) Execute(
	ctx context.Context,
	user domain.User,
) (map[domain.ResourceType]map[domain.Action]any, error) {
	if user.UUID == uuid.Nil || user.OrganizationUUID == nil {
		return nil, errors.New("user UUID or Organization UUID is missing in user context")
	}

	ability, err := uc.abilityFactory.CreateForUser(ctx, user, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting permissions: %w", err)
	}

	return ability.BuildPermissionsMap(), nil
}

// AssignReposInput specifies parameters for updating a user's assigned repositories.
type AssignReposInput struct {
	ActingUserUUID uuid.UUID
	ActingEmail    string
	TargetUserUUID uuid.UUID
	RepoIDs        []string
	TeamUUID       uuid.UUID
	AvailableRepos []string // Optional whitelist of repos configured in the organization
}

// AssignReposUseCase manages repository-scoped access grants for organization members.
type AssignReposUseCase struct {
	userRepo        domain.UserRepository
	permissionsRepo domain.PermissionsRepository
	auditLogEmitter domain.AuditLogEmitter
}

func NewAssignReposUseCase(
	userRepo domain.UserRepository,
	permRepo domain.PermissionsRepository,
	audit domain.AuditLogEmitter,
) *AssignReposUseCase {
	return &AssignReposUseCase{
		userRepo:        userRepo,
		permissionsRepo: permRepo,
		auditLogEmitter: audit,
	}
}

func (uc *AssignReposUseCase) Execute(ctx context.Context, input AssignReposInput) ([]string, error) {
	targetUser, err := uc.userRepo.FindByUUID(ctx, input.TargetUserUUID)
	if err != nil || targetUser == nil {
		return nil, errors.New("user not found")
	}

	// Validate against configured available repos if provided
	validRepoIDs := input.RepoIDs
	if len(input.AvailableRepos) > 0 {
		availSet := make(map[string]struct{}, len(input.AvailableRepos))
		for _, r := range input.AvailableRepos {
			availSet[r] = struct{}{}
		}

		filtered := make([]string, 0, len(input.RepoIDs))
		for _, id := range input.RepoIDs {
			if _, ok := availSet[id]; ok {
				filtered = append(filtered, id)
			}
		}

		if len(input.RepoIDs) > 0 && len(filtered) == 0 {
			return nil, errors.New("none of the provided repository IDs are valid for this organization")
		}
		validRepoIDs = filtered
	}

	// Fetch previous permissions
	existingPerms, _ := uc.permissionsRepo.FindByUserUUID(ctx, input.TargetUserUUID)
	var previousRepoIDs []string
	if existingPerms != nil {
		previousRepoIDs = existingPerms.AssignedRepositoryIDs
	}

	if existingPerms == nil {
		newPerms := domain.Permissions{
			UUID:                  uuid.New(),
			UserUUID:              input.TargetUserUUID,
			AssignedRepositoryIDs: validRepoIDs,
		}
		if _, err := uc.permissionsRepo.Create(ctx, newPerms); err != nil {
			return nil, fmt.Errorf("failed creating permissions record: %w", err)
		}
	} else {
		if _, err := uc.permissionsRepo.Update(ctx, existingPerms.UUID, validRepoIDs); err != nil {
			return nil, fmt.Errorf("failed updating permissions record: %w", err)
		}
	}

	// Compute added and removed IDs for audit trail
	prevSet := make(map[string]struct{}, len(previousRepoIDs))
	for _, id := range previousRepoIDs {
		prevSet[id] = struct{}{}
	}
	nextSet := make(map[string]struct{}, len(validRepoIDs))
	for _, id := range validRepoIDs {
		nextSet[id] = struct{}{}
	}

	var addedIDs, removedIDs []string
	for _, id := range validRepoIDs {
		if _, ok := prevSet[id]; !ok {
			addedIDs = append(addedIDs, id)
		}
	}
	for _, id := range previousRepoIDs {
		if _, ok := nextSet[id]; !ok {
			removedIDs = append(removedIDs, id)
		}
	}

	if (len(addedIDs) > 0 || len(removedIDs) > 0) && uc.auditLogEmitter != nil {
		auditPayload := map[string]any{
			"organization_id": targetUser.OrganizationUUID,
			"team_id":         input.TeamUUID,
			"user_info": map[string]any{
				"user_id":    input.ActingUserUUID,
				"user_email": input.ActingEmail,
			},
			"action_type":        "EDIT",
			"target_user_email":  targetUser.Email,
			"added_repositories": addedIDs,
			"removed_repos":      removedIDs,
		}
		_ = uc.auditLogEmitter.EmitAuditLog(ctx, "USER_REPO_ACCESS", auditPayload)
	}

	return validRepoIDs, nil
}

// GetAssignedReposUseCase retrieves repository IDs assigned to a user.
type GetAssignedReposUseCase struct {
	permissionsRepo domain.PermissionsRepository
}

func NewGetAssignedReposUseCase(repo domain.PermissionsRepository) *GetAssignedReposUseCase {
	return &GetAssignedReposUseCase{permissionsRepo: repo}
}

func (uc *GetAssignedReposUseCase) Execute(ctx context.Context, userUUID uuid.UUID) ([]string, error) {
	if userUUID == uuid.Nil {
		return []string{}, nil
	}

	perms, err := uc.permissionsRepo.FindByUserUUID(ctx, userUUID)
	if err != nil || perms == nil {
		return []string{}, nil
	}

	return perms.AssignedRepositoryIDs, nil
}
