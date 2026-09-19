package channels_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
	"github.com/scandrix/backend/internal/notifications/infrastructure/adapters/channels"
)

func TestSlackAndDiscordAndWebhookAdapters(t *testing.T) {
	ctx := context.Background()

	var receivedSlackBody string
	slackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedSlackBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer slackServer.Close()

	var receivedDiscordBody string
	discordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedDiscordBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer discordServer.Close()

	var receivedWebhookBody string
	var receivedSignature string
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-ScanDrix-Signature")
		b, _ := io.ReadAll(r.Body)
		receivedWebhookBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	deliveryCtx := contracts.DeliveryContext{
		DeliveryID:     uuid.New().String(),
		UserID:         uuid.New().String(),
		UserEmail:      "team@scandrix.dev",
		UserRole:       "admin",
		OrganizationID: uuid.New().String(),
		Event:          catalog.EventReviewFailed,
		Criticality:    enums.CriticalityTransactional,
		Title:          "Code review failed",
		Body:           "Drixy could not complete analysis due to syntax error in Go file.",
		CtaURL:         "https://app.scandrix.dev/reviews/123",
		Category:       "review",
		Metadata: map[string]interface{}{
			"slackWebhookUrl":   slackServer.URL,
			"discordWebhookUrl": discordServer.URL,
			"webhookUrl":        webhookServer.URL,
			"webhookSecret":     "test-secret-key-123",
		},
		CorrelationID: "corr-xyz",
	}

	// 1. Slack
	slackAdapter := channels.NewSlackChannelAdapter("")
	if err := slackAdapter.Deliver(ctx, deliveryCtx); err != nil {
		t.Fatalf("slack delivery failed: %v", err)
	}
	if !strings.Contains(receivedSlackBody, "Code review failed") || !strings.Contains(receivedSlackBody, "ScanDrix Code Review") {
		t.Fatalf("unexpected slack payload: %s", receivedSlackBody)
	}

	// 2. Discord
	discordAdapter := channels.NewDiscordChannelAdapter("")
	if err := discordAdapter.Deliver(ctx, deliveryCtx); err != nil {
		t.Fatalf("discord delivery failed: %v", err)
	}
	if !strings.Contains(receivedDiscordBody, "ScanDrix Code Review") || !strings.Contains(receivedDiscordBody, "Code review failed") {
		t.Fatalf("unexpected discord payload: %s", receivedDiscordBody)
	}

	// 3. Webhook
	webhookAdapter := channels.NewWebhookChannelAdapter("", "")
	if err := webhookAdapter.Deliver(ctx, deliveryCtx); err != nil {
		t.Fatalf("webhook delivery failed: %v", err)
	}
	if !strings.HasPrefix(receivedSignature, "sha256=") {
		t.Fatalf("expected HMAC-SHA256 signature header, got: %s", receivedSignature)
	}
	if !strings.Contains(receivedWebhookBody, "Code review failed") {
		t.Fatalf("unexpected webhook payload: %s", receivedWebhookBody)
	}
}
