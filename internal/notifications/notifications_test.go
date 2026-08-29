package notifications_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications"
)

type mockSender struct {
	sentCount int
}

func (m *mockSender) Send(ctx context.Context, event notifications.NotificationEvent, recipient string) error {
	m.sentCount++
	return nil
}

func TestNotificationRateLimiter(t *testing.T) {
	limiter := notifications.NewNotificationRateLimiter(5 * time.Minute)
	wsID := uuid.New()

	// 1. First event -> allowed
	if !limiter.Allow(wsID, "repo-123", "critical_vulnerability") {
		t.Fatal("expected first alert to be allowed")
	}

	// 2. Immediate duplicate -> suppressed
	if limiter.Allow(wsID, "repo-123", "critical_vulnerability") {
		t.Fatal("expected immediate duplicate alert to be suppressed")
	}

	// 3. Different event type on same repo -> allowed
	if !limiter.Allow(wsID, "repo-123", "review_completed") {
		t.Fatal("expected different event type to be allowed")
	}

	// 4. Reset allows key again
	limiter.Reset(wsID, "repo-123", "critical_vulnerability")
	if !limiter.Allow(wsID, "repo-123", "critical_vulnerability") {
		t.Fatal("expected alert to be allowed after reset")
	}
}

func TestRoutingRuleService(t *testing.T) {
	router := notifications.NewRoutingRuleService()
	wsID := uuid.New()

	// Add rule: Critical events go to Slack and Email
	router.AddRule(notifications.NotificationRoutingRule{
		ID:             uuid.New(),
		WorkspaceID:    wsID,
		MinCriticality: notifications.CriticalityHigh,
		Channels:       []notifications.ChannelType{notifications.ChannelSlack, notifications.ChannelEmail},
		Enabled:        true,
	})

	// Add rule: All events go to In-App
	router.AddRule(notifications.NotificationRoutingRule{
		ID:             uuid.New(),
		WorkspaceID:    wsID,
		MinCriticality: notifications.CriticalityLow,
		Channels:       []notifications.ChannelType{notifications.ChannelInApp},
		Enabled:        true,
	})

	// 1. Critical event -> matches Slack, Email, and In-App
	critEvent := notifications.NotificationEvent{
		WorkspaceID: wsID,
		Criticality: notifications.CriticalityCritical,
	}
	critChannels := router.ResolveChannels(critEvent)
	if len(critChannels) != 3 {
		t.Fatalf("expected 3 channels for critical event, got %d", len(critChannels))
	}

	// 2. Low event -> matches only In-App
	lowEvent := notifications.NotificationEvent{
		WorkspaceID: wsID,
		Criticality: notifications.CriticalityLow,
	}
	lowChannels := router.ResolveChannels(lowEvent)
	if len(lowChannels) != 1 || lowChannels[0] != notifications.ChannelInApp {
		t.Fatalf("expected only in-app channel for low event, got %+v", lowChannels)
	}
}

func TestNotificationDispatcher(t *testing.T) {
	router := notifications.NewRoutingRuleService()
	limiter := notifications.NewNotificationRateLimiter(1 * time.Minute)
	dispatcher := notifications.NewNotificationDispatcher(router, limiter)

	slackMock := &mockSender{}
	dispatcher.RegisterSender(notifications.ChannelSlack, slackMock)

	wsID := uuid.New()
	router.AddRule(notifications.NotificationRoutingRule{
		ID:             uuid.New(),
		WorkspaceID:    wsID,
		MinCriticality: notifications.CriticalityCritical,
		Channels:       []notifications.ChannelType{notifications.ChannelSlack},
		Enabled:        true,
	})

	ctx := context.Background()
	event := notifications.NotificationEvent{
		ID:           uuid.New(),
		WorkspaceID:  wsID,
		RepositoryID: uuid.New(),
		EventType:    "critical_security_finding",
		Criticality:  notifications.CriticalityCritical,
		Title:        "CWE-89 SQL Injection Detected",
		Message:      "Unsanitized input in user login query",
		Recipient:    "#security-alerts",
		CreatedAt:    time.Now().UTC(),
	}

	// 1. Dispatch event
	records, err := dispatcher.Dispatch(ctx, event)
	if err != nil {
		t.Fatalf("failed dispatching notification: %v", err)
	}
	if len(records) != 1 || records[0].Status != notifications.DeliverySent {
		t.Fatalf("unexpected delivery record: %+v", records)
	}
	if slackMock.sentCount != 1 {
		t.Fatalf("expected slack sender to be called once, got %d", slackMock.sentCount)
	}

	// 2. Dispatch duplicate immediately -> Rate limited
	dupRecords, err := dispatcher.Dispatch(ctx, event)
	if err != nil {
		t.Fatalf("failed dispatching duplicate: %v", err)
	}
	if len(dupRecords) != 1 || dupRecords[0].Status != notifications.DeliveryRateLimited {
		t.Fatalf("expected rate limited record, got %+v", dupRecords)
	}
	// Sender count should still be 1
	if slackMock.sentCount != 1 {
		t.Fatalf("expected slack sender count to remain 1, got %d", slackMock.sentCount)
	}
}
