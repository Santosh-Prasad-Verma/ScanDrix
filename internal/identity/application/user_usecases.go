package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/scandrix/backend/internal/auth"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// GetUserUseCase retrieves the full profile of an authenticated user.
type GetUserUseCase struct {
	userRepo domain.UserRepository
}

func NewGetUserUseCase(repo domain.UserRepository) *GetUserUseCase {
	return &GetUserUseCase{userRepo: repo}
}

func (uc *GetUserUseCase) Execute(ctx context.Context, userUUID uuid.UUID) (*domain.User, error) {
	if userUUID == uuid.Nil {
		return nil, errors.New("invalid user ID")
	}

	user, err := uc.userRepo.FindByUUID(ctx, userUUID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	safe := user.UserSafeView()
	return &safe, nil
}

// CheckUserEmailUseCase checks whether an email address is already registered.
type CheckUserEmailUseCase struct {
	userRepo domain.UserRepository
}

func NewCheckUserEmailUseCase(repo domain.UserRepository) *CheckUserEmailUseCase {
	return &CheckUserEmailUseCase{userRepo: repo}
}

func (uc *CheckUserEmailUseCase) Execute(ctx context.Context, email string) (bool, error) {
	count, err := uc.userRepo.Count(ctx, map[string]any{"email": email})
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, ErrDuplicateEmail
	}
	return true, nil
}

// AcceptUserInvitationInput contains credentials and profile data for accepting an invitation.
type AcceptUserInvitationInput struct {
	UserUUID uuid.UUID `json:"uuid"`
	Password string    `json:"password"`
	Name     string    `json:"name"`
	Phone    string    `json:"phone,omitempty"`
}

// AcceptUserInvitationUseCase finalizes account setup for an invited organization member.
type AcceptUserInvitationUseCase struct {
	userRepo        domain.UserRepository
	profileRepo     domain.ProfileRepository
	passwordService domain.PasswordService
	auditEmitter    domain.AuditLogEmitter
}

func NewAcceptUserInvitationUseCase(
	userRepo domain.UserRepository,
	profileRepo domain.ProfileRepository,
	pwService domain.PasswordService,
	audit domain.AuditLogEmitter,
) *AcceptUserInvitationUseCase {
	return &AcceptUserInvitationUseCase{
		userRepo:        userRepo,
		profileRepo:     profileRepo,
		passwordService: pwService,
		auditEmitter:    audit,
	}
}

func (uc *AcceptUserInvitationUseCase) Execute(ctx context.Context, input AcceptUserInvitationInput) (*domain.User, error) {
	user, err := uc.userRepo.FindByUUID(ctx, input.UserUUID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	// AUDIT_REMEDIATION.md F-21: the invitation path had no policy check. The
	// invited user's email and name are the identifiers to avoid.
	if err := auth.ValidatePassword(input.Password, user.Email, input.Name); err != nil {
		return nil, fmt.Errorf("password rejected: %w", err)
	}

	hashedPassword, err := uc.passwordService.HashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("failed hashing password: %w", err)
	}

	updatedUser, err := uc.userRepo.Update(ctx, user.UUID, map[string]any{
		"status":   string(domain.UserStatusActive),
		"password": hashedPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("failed activating user: %w", err)
	}

	_ = uc.profileRepo.UpdateByUserID(ctx, user.UUID, domain.UserProfile{
		Name:  input.Name,
		Phone: input.Phone,
	})

	if uc.auditEmitter != nil {
		_ = uc.auditEmitter.EmitAuditLog(ctx, "USER_INVITATION_ACCEPTED", map[string]any{
			"user_id":         user.UUID,
			"email":           user.Email,
			"organization_id": user.OrganizationUUID,
		})
	}

	safe := updatedUser.UserSafeView()
	return &safe, nil
}

// DeleteUserUseCase purges a user and their cascaded relations across the workspace.
type DeleteUserUseCase struct {
	userRepo domain.UserRepository
}

func NewDeleteUserUseCase(repo domain.UserRepository) *DeleteUserUseCase {
	return &DeleteUserUseCase{userRepo: repo}
}

func (uc *DeleteUserUseCase) Execute(ctx context.Context, userUUID uuid.UUID) error {
	count, err := uc.userRepo.Count(ctx, map[string]any{"uuid": userUUID})
	if err != nil || count == 0 {
		return errors.New("user not found")
	}

	return uc.userRepo.Delete(ctx, userUUID)
}

// UpdateAnotherUserInput specifies modifications made to a colleague's account.
type UpdateAnotherUserInput struct {
	ActingUserUUID   uuid.UUID
	ActingEmail      string
	TargetUserUUID   uuid.UUID
	OrganizationUUID uuid.UUID
	NewRole          *domain.Role
	NewStatus        *domain.UserStatus
}

// UpdateAnotherUserUseCase modifies permissions and statuses of organization members.
type UpdateAnotherUserUseCase struct {
	userRepo     domain.UserRepository
	auditEmitter domain.AuditLogEmitter
	notifEmitter domain.NotificationEmitter
}

func NewUpdateAnotherUserUseCase(
	userRepo domain.UserRepository,
	audit domain.AuditLogEmitter,
	notif domain.NotificationEmitter,
) *UpdateAnotherUserUseCase {
	return &UpdateAnotherUserUseCase{
		userRepo:     userRepo,
		auditEmitter: audit,
		notifEmitter: notif,
	}
}

func (uc *UpdateAnotherUserUseCase) Execute(ctx context.Context, input UpdateAnotherUserInput) (*domain.User, error) {
	targetUser, err := uc.userRepo.FindByUUID(ctx, input.TargetUserUUID)
	if err != nil || targetUser == nil {
		return nil, errors.New("target user not found")
	}

	if targetUser.OrganizationUUID == nil || *targetUser.OrganizationUUID != input.OrganizationUUID {
		return nil, errors.New("target user is not a member of the organization")
	}

	previousRole := targetUser.Role
	updates := make(map[string]any)

	if input.NewRole != nil {
		updates["role"] = string(*input.NewRole)
	}
	if input.NewStatus != nil {
		updates["status"] = string(*input.NewStatus)
	}

	updatedUser, err := uc.userRepo.Update(ctx, targetUser.UUID, updates)
	if err != nil {
		return nil, fmt.Errorf("failed updating target user: %w", err)
	}

	if input.NewRole != nil && previousRole != *input.NewRole {
		if uc.auditEmitter != nil {
			_ = uc.auditEmitter.EmitAuditLog(ctx, "USER_ROLE_CHANGE", map[string]any{
				"organization_id":   input.OrganizationUUID,
				"acting_user_id":    input.ActingUserUUID,
				"acting_email":      input.ActingEmail,
				"target_user_email": targetUser.Email,
				"previous_role":     previousRole,
				"new_role":          *input.NewRole,
			})
		}

		if uc.notifEmitter != nil {
			_ = uc.notifEmitter.Emit(ctx, "ORG_ROLE_CHANGED", map[string]any{
				"affected_user_email": targetUser.Email,
				"previous_role":       string(previousRole),
				"new_role":            string(*input.NewRole),
				"changed_by":          input.ActingEmail,
				"organization_name":   targetUser.OrganizationName,
			}, &input.OrganizationUUID, nil)
		}
	}

	safe := updatedUser.UserSafeView()
	return &safe, nil
}

// InviteDataResult contains basic organization metadata for an invite screen.
type InviteDataResult struct {
	UUID             uuid.UUID `json:"uuid"`
	Email            string    `json:"email"`
	OrganizationName string    `json:"organization_name"`
}

// InviteDataUseCase looks up public invite information for invitation landing pages.
type InviteDataUseCase struct {
	userRepo domain.UserRepository
}

func NewInviteDataUseCase(repo domain.UserRepository) *InviteDataUseCase {
	return &InviteDataUseCase{userRepo: repo}
}

func (uc *InviteDataUseCase) Execute(ctx context.Context, inviteUUID uuid.UUID) (*InviteDataResult, error) {
	if inviteUUID == uuid.Nil {
		return nil, errors.New("invalid invitation ID")
	}

	user, err := uc.userRepo.FindOne(ctx, map[string]any{
		"uuid":   inviteUUID,
		"status": string(domain.UserStatusPending),
	})
	if err != nil || user == nil {
		return nil, errors.New("invitation not found or expired")
	}

	return &InviteDataResult{
		UUID:             user.UUID,
		Email:            user.Email,
		OrganizationName: user.OrganizationName,
	}, nil
}
