package organization_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization"
)

func TestWorkspaceServiceSeatQuotasAndInvites(t *testing.T) {
	svc := organization.NewWorkspaceService()

	// 1. Create Workspace with 2 seats limit
	ws, err := svc.CreateWorkspace("acme-corp", "Acme Corporation", "growth", 2)
	if err != nil {
		t.Fatalf("failed creating workspace: %v", err)
	}

	// 2. Reject duplicate slug
	_, errDup := svc.CreateWorkspace("acme-corp", "Acme Clone", "starter", 5)
	if errDup == nil {
		t.Fatal("expected error for duplicate workspace slug")
	}

	// 3. Issue Invite 1 (Seat 1 used by owner, 1 left -> Allowed)
	inv1, err := svc.InviteMember(ws.ID, "bob@acme.com", "developer")
	if err != nil {
		t.Fatalf("failed inviting first member: %v", err)
	}

	// 4. Accept Invite 1 -> Seats used reaches 2
	_, err = svc.AcceptInvitation(inv1.InviteToken)
	if err != nil {
		t.Fatalf("failed accepting invitation: %v", err)
	}

	// 5. Issue Invite 2 -> Should fail because seats are exhausted (2/2)
	_, errExhausted := svc.InviteMember(ws.ID, "charlie@acme.com", "developer")
	if errExhausted == nil {
		t.Fatal("expected error inviting member beyond seat quota")
	}
}

func TestTeamServiceAutoAssignReviewers(t *testing.T) {
	teamSvc := organization.NewTeamService()
	wsID := uuid.New()

	// 1. Create Team with least_busy auto-assign
	team, err := teamSvc.CreateTeam(wsID, "Platform Security Team", "least_busy")
	if err != nil {
		t.Fatalf("failed creating team: %v", err)
	}

	userA := uuid.New()
	userB := uuid.New()
	userC := uuid.New()

	_, _ = teamSvc.AddMember(team.ID, userA, "lead")
	_, _ = teamSvc.AddMember(team.ID, userB, "member")
	_, _ = teamSvc.AddMember(team.ID, userC, "member")

	// 2. Assign review, excluding userA (the PR author)
	reviewer1, err := teamSvc.AutoAssignReviewer(team.ID, &userA)
	if err != nil {
		t.Fatalf("failed auto-assigning reviewer: %v", err)
	}
	if reviewer1.UserID == userA {
		t.Fatal("PR author should never be assigned as reviewer")
	}

	// 3. Next review assignment should pick the least busy other member
	reviewer2, err := teamSvc.AutoAssignReviewer(team.ID, &userA)
	if err != nil {
		t.Fatalf("failed auto-assigning second reviewer: %v", err)
	}
	if reviewer2.UserID == reviewer1.UserID {
		t.Fatal("least_busy should balance load across available members")
	}
}
