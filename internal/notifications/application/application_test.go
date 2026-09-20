package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/application"
	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
	"github.com/scandrix/backend/internal/notifications/domain/recipient"
	"github.com/scandrix/backend/internal/notifications/infrastructure/repositories"
)

type dummyChannelAdapter struct {
	channel   enums.Channel
	delivered []contracts.DeliveryContext
}

func (d *dummyChannelAdapter) Channel() enums.Channel {
	return d.channel
}

func (d *dummyChannelAdapter) Deliver(ctx context.Context, deliveryCtx contracts.DeliveryContext) error {
	d.delivered = append(d.delivered, deliveryCtx)
	return nil
}

type dummyUserLookup struct{}

func (d *dummyUserLookup) FindUserByEmail(ctx context.Context, email string, orgID string) (*application.UserProfileRef, error) {
	return &application.UserProfileRef{UserID: uuid.New().String(), Email: email, Role: catalog.RoleContributor}, nil
}
func (d *dummyUserLookup) FindUserByID(ctx context.Context, userID string) (*application.UserProfileRef, error) {
	return &application.UserProfileRef{UserID: userID, Email: "user@scandrix.dev", Role: catalog.RoleContributor}, nil
}
func (d *dummyUserLookup) FindUsersByRole(ctx context.Context, orgID string, role string) ([]*application.UserProfileRef, error) {
	return []*application.UserProfileRef{{UserID: uuid.New().String(), Email: "admin@scandrix.dev", Role: role}}, nil
}
func (d *dummyUserLookup) FindAllOrgMembers(ctx context.Context, orgID string) ([]*application.UserProfileRef, error) {
	return []*application.UserProfileRef{
		{UserID: uuid.New().String(), Email: "dev@scandrix.dev", Role: catalog.RoleContributor},
		{UserID: uuid.New().String(), Email: "owner@scandrix.dev", Role: catalog.RoleOwner},
	}, nil
}

func TestDispatcherMultiChannelExecution(t *testing.T) {
	ctx := context.Background()
	delRepo := repositories.NewMemoryDeliveryRepository()
	ruleRepo := repositories.NewMemoryRoutingRuleRepository()
	lookup := &dummyUserLookup{}
	sse := application.NewNotificationSseService()

	slackAd := &dummyChannelAdapter{channel: enums.ChannelSlack}
	emailAd := &dummyChannelAdapter{channel: enums.ChannelEmail}
	inAppAd := &dummyChannelAdapter{channel: enums.ChannelInApp}
	discordAd := &dummyChannelAdapter{channel: enums.ChannelDiscord}
	webhookAd := &dummyChannelAdapter{channel: enums.ChannelWebhook}

	adapters := []contracts.ChannelAdapter{slackAd, emailAd, inAppAd, discordAd, webhookAd}

	dispatcher := application.NewNotificationDispatcherService(
		adapters,
		delRepo,
		ruleRepo,
		lookup,
		sse,
		nil,
	)

	orgID := uuid.New().String()
	userID := uuid.New().String()

	msg := application.NotificationMessage{
		Event: catalog.EventReviewAutoApproved,
		Payload: map[string]interface{}{
			"repoName": "scandrix/engine",
			"prUrl":    "https://github.com/scandrix/engine/pull/1",
		},
		OrganizationID: orgID,
		Recipients: []recipient.Recipient{
			recipient.ByUser(userID),
		},
		CorrelationID: "test-corr-abc",
	}

	err := dispatcher.Dispatch(ctx, msg)
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// Email and In-App are default channels for EventReviewAutoApproved
	if len(emailAd.delivered) != 1 {
		t.Fatalf("expected 1 email delivery, got %d", len(emailAd.delivered))
	}
	if len(inAppAd.delivered) != 1 {
		t.Fatalf("expected 1 in-app delivery, got %d", len(inAppAd.delivered))
	}
}

func TestSSEServiceSubscriptionAndPush(t *testing.T) {
	sse := application.NewNotificationSseService()
	userID := uuid.New().String()
	ch := make(chan []byte, 10)

	sse.AddConnection(userID, ch)

	sse.PushEvent(userID, "notification", map[string]interface{}{
		"title": "Rule Synced",
	})

	select {
	case msg := <-ch:
		msgStr := string(msg)
		if !strings.Contains(msgStr, "event: notification") || !strings.Contains(msgStr, "Rule Synced") {
			t.Fatalf("unexpected SSE packet: %s", msgStr)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for SSE push")
	}

	sse.RemoveConnection(userID, ch)
}
