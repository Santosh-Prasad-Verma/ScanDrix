package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/entities"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
	"github.com/scandrix/backend/internal/notifications/infrastructure/repositories"
)

func TestMemoryRepositories(t *testing.T) {
	ctx := context.Background()
	delRepo := repositories.NewMemoryDeliveryRepository()
	userNotifRepo := repositories.NewMemoryUserNotificationRepository(delRepo)
	ruleRepo := repositories.NewMemoryRoutingRuleRepository()

	orgID := uuid.New()
	userID := uuid.New()

	// 1. Test Delivery Repo
	del := entities.NewDelivery(
		orgID,
		"review.auto_approved",
		enums.CriticalityInformational,
		enums.ChannelInApp,
		"Test Title",
		"Test Body",
		"review",
		"corr-1",
	)
	del.RecipientUserID = &userID

	err := delRepo.Create(ctx, del)
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	found, err := delRepo.FindByID(ctx, del.UUID)
	if err != nil || found == nil {
		t.Fatalf("expected to find delivery by id, got: %v", err)
	}

	// 2. Test UserNotification Repo
	un := entities.NewUserNotification(userID, del.UUID)
	err = userNotifRepo.Create(ctx, un)
	if err != nil {
		t.Fatalf("failed to create user notification: %v", err)
	}

	unread, err := userNotifRepo.CountUnread(ctx, userID)
	if err != nil || unread != 1 {
		t.Fatalf("expected 1 unread notification, got %d", unread)
	}

	list, total, err := userNotifRepo.FindByUser(ctx, userID, 10, 0, false)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("expected 1 item in list, got total %d, count %d", total, len(list))
	}
	if list[0].Delivery.Title != "Test Title" {
		t.Fatalf("expected joined title 'Test Title', got '%s'", list[0].Delivery.Title)
	}

	// Mark read
	err = userNotifRepo.MarkAsRead(ctx, un.UUID, userID)
	if err != nil {
		t.Fatalf("mark read failed: %v", err)
	}
	unreadAfter, _ := userNotifRepo.CountUnread(ctx, userID)
	if unreadAfter != 0 {
		t.Fatalf("expected 0 unread after mark, got %d", unreadAfter)
	}

	// 3. Test Routing Rule Repo
	category := "review"
	rule := entities.NewRoutingRule(
		orgID,
		"review.auto_approved",
		&category,
		"contributor",
		map[string]bool{"email": true, "in_app": true},
	)
	err = ruleRepo.Upsert(ctx, rule)
	if err != nil {
		t.Fatalf("failed to upsert rule: %v", err)
	}

	resolved, err := ruleRepo.Resolve(ctx, orgID, "review.auto_approved", "contributor")
	if err != nil || resolved == nil {
		t.Fatalf("failed to resolve rule: %v", err)
	}
	if !resolved.Channels["email"] {
		t.Fatal("expected email channel to be enabled in resolved rule")
	}

	// Test retry batch claiming
	past := time.Now().UTC().Add(-1 * time.Minute)
	_ = delRepo.ScheduleRetry(ctx, del.UUID, past, "temporary connection error")

	batch, err := delRepo.ClaimRetryBatch(ctx, 10, "worker-test-1")
	if err != nil || len(batch) != 1 {
		t.Fatalf("expected 1 item in claimed retry batch, got %d", len(batch))
	}
	if *batch[0].LockedBy != "worker-test-1" {
		t.Fatalf("expected lockedBy 'worker-test-1', got '%v'", batch[0].LockedBy)
	}
}
