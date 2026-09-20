package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/application"
	"github.com/scandrix/backend/internal/identity/domain"
	"github.com/scandrix/backend/internal/identity/infrastructure"
)

func TestUpdateAnotherUserUseCase_OrgRoleChangedEmit(t *testing.T) {
	ctx := context.Background()
	orgUUID := uuid.New()
	actingUUID := uuid.New()
	targetUUID := uuid.New()

	setup := func(notifFail bool) (*application.UpdateAnotherUserUseCase, domain.UserRepository, *mockNotificationEmitter, *mockAuditLogEmitter) {
		userRepo := infrastructure.NewInMemoryUserRepository()
		auditEmitter := &mockAuditLogEmitter{}
		notifEmitter := &mockNotificationEmitter{shouldFail: notifFail}

		// Acting user
		_, _ = userRepo.Create(ctx, domain.User{
			UUID:             actingUUID,
			Email:            "admin@acme.com",
			Role:             domain.RoleOwner,
			OrganizationUUID: &orgUUID,
			OrganizationName: "Acme Inc",
			Status:           domain.UserStatusActive,
		})

		// Target user
		_, _ = userRepo.Create(ctx, domain.User{
			UUID:             targetUUID,
			Email:            "target@acme.com",
			Role:             domain.RoleContributor,
			OrganizationUUID: &orgUUID,
			OrganizationName: "Acme Inc",
			Status:           domain.UserStatusActive,
		})

		uc := application.NewUpdateAnotherUserUseCase(userRepo, auditEmitter, notifEmitter)
		return uc, userRepo, notifEmitter, auditEmitter
	}

	t.Run("emits org.role_changed when role transitions to a different value", func(t *testing.T) {
		uc, _, notif, audit := setup(false)
		newRole := domain.RoleOwner

		res, err := uc.Execute(ctx, application.UpdateAnotherUserInput{
			ActingUserUUID:   actingUUID,
			ActingEmail:      "admin@acme.com",
			TargetUserUUID:   targetUUID,
			OrganizationUUID: orgUUID,
			NewRole:          &newRole,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Role != domain.RoleOwner {
			t.Fatalf("expected role OWNER, got %s", res.Role)
		}

		if len(notif.emittedEvents) == 0 {
			t.Fatal("expected ORG_ROLE_CHANGED event to be emitted")
		}
		if notif.emittedEvents[0] != "ORG_ROLE_CHANGED" {
			t.Fatalf("expected ORG_ROLE_CHANGED, got %s", notif.emittedEvents[0])
		}

		payload := notif.emittedPayloads[0]
		if payload["affected_user_email"] != "target@acme.com" {
			t.Fatalf("expected target@acme.com, got %v", payload["affected_user_email"])
		}
		if payload["previous_role"] != string(domain.RoleContributor) {
			t.Fatalf("expected previous role contributor, got %v", payload["previous_role"])
		}
		if payload["new_role"] != string(domain.RoleOwner) {
			t.Fatalf("expected new role owner, got %v", payload["new_role"])
		}
		if payload["changed_by"] != "admin@acme.com" {
			t.Fatalf("expected changed_by admin@acme.com, got %v", payload["changed_by"])
		}

		// Also check audit log
		if len(audit.events) == 0 || audit.events[0] != "USER_ROLE_CHANGE" {
			t.Fatal("expected audit log USER_ROLE_CHANGE")
		}
	})

	t.Run("does NOT emit when role is unchanged", func(t *testing.T) {
		uc, _, notif, audit := setup(false)
		sameRole := domain.RoleContributor

		_, err := uc.Execute(ctx, application.UpdateAnotherUserInput{
			ActingUserUUID:   actingUUID,
			ActingEmail:      "admin@acme.com",
			TargetUserUUID:   targetUUID,
			OrganizationUUID: orgUUID,
			NewRole:          &sameRole,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(notif.emittedEvents) != 0 {
			t.Fatal("did not expect notification when role is unchanged")
		}
		if len(audit.events) != 0 {
			t.Fatal("did not expect audit log when role is unchanged")
		}
	})

	t.Run("does NOT emit when role is not in the update payload", func(t *testing.T) {
		uc, _, notif, audit := setup(false)
		newStatus := domain.UserStatusInactive

		_, err := uc.Execute(ctx, application.UpdateAnotherUserInput{
			ActingUserUUID:   actingUUID,
			ActingEmail:      "admin@acme.com",
			TargetUserUUID:   targetUUID,
			OrganizationUUID: orgUUID,
			NewStatus:        &newStatus,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(notif.emittedEvents) != 0 {
			t.Fatal("did not expect notification when role is not updated")
		}
		if len(audit.events) != 0 {
			t.Fatal("did not expect audit log when role is not updated")
		}
	})

	t.Run("still emits the audit-log event when role changes (existing behaviour preserved)", func(t *testing.T) {
		uc, _, _, audit := setup(false)
		newRole := domain.RoleRepoAdmin

		_, err := uc.Execute(ctx, application.UpdateAnotherUserInput{
			ActingUserUUID:   actingUUID,
			ActingEmail:      "admin@acme.com",
			TargetUserUUID:   targetUUID,
			OrganizationUUID: orgUUID,
			NewRole:          &newRole,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(audit.events) == 0 {
			t.Fatal("expected audit log event")
		}
		if audit.events[0] != "USER_ROLE_CHANGE" {
			t.Fatalf("expected USER_ROLE_CHANGE, got %s", audit.events[0])
		}
	})

	t.Run("swallows notification failures so the update still returns", func(t *testing.T) {
		uc, _, _, _ := setup(true) // will fail emit
		newRole := domain.RoleOwner

		res, err := uc.Execute(ctx, application.UpdateAnotherUserInput{
			ActingUserUUID:   actingUUID,
			ActingEmail:      "admin@acme.com",
			TargetUserUUID:   targetUUID,
			OrganizationUUID: orgUUID,
			NewRole:          &newRole,
		})
		if err != nil {
			t.Fatalf("expected update to succeed even when notification fails, got: %v", err)
		}
		if res == nil || res.Role != domain.RoleOwner {
			t.Fatal("expected user to be updated successfully")
		}
	})
}

func TestUserManagementUseCases(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	profileRepo := infrastructure.NewInMemoryProfileRepository()
	pwService := infrastructure.NewBcryptPasswordService(10)
	auditEmitter := &mockAuditLogEmitter{}

	getUC := application.NewGetUserUseCase(userRepo)
	checkEmailUC := application.NewCheckUserEmailUseCase(userRepo)
	acceptInviteUC := application.NewAcceptUserInvitationUseCase(userRepo, profileRepo, pwService, auditEmitter)
	deleteUC := application.NewDeleteUserUseCase(userRepo)
	inviteDataUC := application.NewInviteDataUseCase(userRepo)

	orgID := uuid.New()
	userUUID := uuid.New()

	// Create pending user
	_, err := userRepo.Create(ctx, domain.User{
		UUID:             userUUID,
		Email:            "invitee@scandrix.dev",
		OrganizationUUID: &orgID,
		OrganizationName: "Test Org",
		Status:           domain.UserStatusPending,
		Role:             domain.RoleContributor,
	})
	if err != nil {
		t.Fatalf("failed creating user: %v", err)
	}

	// 1. Invite Data
	inviteData, err := inviteDataUC.Execute(ctx, userUUID)
	if err != nil {
		t.Fatalf("failed getting invite data: %v", err)
	}
	if inviteData.Email != "invitee@scandrix.dev" || inviteData.OrganizationName != "Test Org" {
		t.Fatalf("unexpected invite data: %+v", inviteData)
	}

	// 2. Accept Invitation
	accepted, err := acceptInviteUC.Execute(ctx, application.AcceptUserInvitationInput{
		UserUUID: userUUID,
		Password: "SecurePassword987!",
		Name:     "Invited Developer",
		Phone:    "+15550199",
	})
	if err != nil {
		t.Fatalf("failed accepting invite: %v", err)
	}
	if accepted.Status != domain.UserStatusActive {
		t.Fatalf("expected active status, got: %s", accepted.Status)
	}

	// 3. Get User
	user, err := getUC.Execute(ctx, userUUID)
	if err != nil {
		t.Fatalf("failed getting user: %v", err)
	}
	if user.Email != "invitee@scandrix.dev" {
		t.Fatalf("unexpected email: %s", user.Email)
	}

	// 4. Check Email (should now exist)
	available, err := checkEmailUC.Execute(ctx, "invitee@scandrix.dev")
	if available || err != application.ErrDuplicateEmail {
		t.Fatalf("expected duplicate email error, got available=%v, err=%v", available, err)
	}

	// Available email check
	available2, err := checkEmailUC.Execute(ctx, "nonexistent@scandrix.dev")
	if !available2 || err != nil {
		t.Fatalf("expected email to be available, got available=%v, err=%v", available2, err)
	}

	// 5. Delete User
	err = deleteUC.Execute(ctx, userUUID)
	if err != nil {
		t.Fatalf("failed deleting user: %v", err)
	}

	// Verify deleted
	_, err = getUC.Execute(ctx, userUUID)
	if err == nil {
		t.Fatal("expected error getting deleted user")
	}
}
