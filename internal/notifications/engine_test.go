package notifications_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications"
	"github.com/scandrix/backend/internal/notifications/application"
	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
	"github.com/scandrix/backend/internal/notifications/domain/recipient"
	"github.com/scandrix/backend/internal/notifications/infrastructure/adapters/channels"
	"github.com/scandrix/backend/internal/notifications/infrastructure/adapters/email_providers"
)

type mockUserLookup struct {
	usersByEmail map[string]*application.UserProfileRef
	usersByID    map[string]*application.UserProfileRef
	members      []*application.UserProfileRef
}

func (m *mockUserLookup) FindUserByEmail(ctx context.Context, email string, orgID string) (*application.UserProfileRef, error) {
	return m.usersByEmail[email], nil
}

func (m *mockUserLookup) FindUserByID(ctx context.Context, userID string) (*application.UserProfileRef, error) {
	return m.usersByID[userID], nil
}

func (m *mockUserLookup) FindUsersByRole(ctx context.Context, orgID string, role string) ([]*application.UserProfileRef, error) {
	var results []*application.UserProfileRef
	for _, u := range m.members {
		if u.Role == role {
			results = append(results, u)
		}
	}
	return results, nil
}

func (m *mockUserLookup) FindAllOrgMembers(ctx context.Context, orgID string) ([]*application.UserProfileRef, error) {
	return m.members, nil
}

func setupTestEngine() (*notifications.Engine, *email_providers.MemoryEmailProvider, *mockUserLookup) {
	emailMock := email_providers.NewMemoryEmailProvider()
	user1ID := uuid.New().String()
	user2ID := uuid.New().String()

	lookup := &mockUserLookup{
		usersByEmail: map[string]*application.UserProfileRef{
			"dev@scandrix.dev": {
				UserID: user1ID,
				Email:  "dev@scandrix.dev",
				Role:   catalog.RoleContributor,
			},
			"owner@scandrix.dev": {
				UserID: user2ID,
				Email:  "owner@scandrix.dev",
				Role:   catalog.RoleOwner,
			},
		},
		usersByID: make(map[string]*application.UserProfileRef),
		members: []*application.UserProfileRef{
			{UserID: user1ID, Email: "dev@scandrix.dev", Role: catalog.RoleContributor},
			{UserID: user2ID, Email: "owner@scandrix.dev", Role: catalog.RoleOwner},
		},
	}
	lookup.usersByID[user1ID] = lookup.usersByEmail["dev@scandrix.dev"]
	lookup.usersByID[user2ID] = lookup.usersByEmail["owner@scandrix.dev"]

	engine := notifications.NewEngine(
		nil, // in-memory repo
		nil, // in-memory rate limiter
		emailMock,
		lookup,
		nil, // local dispatch
		notifications.Config{
			WebURL: "https://app.scandrix.dev",
		},
	)

	return engine, emailMock, lookup
}

func TestCatalogAndDefaultsParity(t *testing.T) {
	// Verify that all 20+ events are registered
	expectedEvents := []catalog.Event{
		catalog.EventAuthEmailConfirmation,
		catalog.EventAuthForgotPassword,
		catalog.EventTeamMemberInvited,
		catalog.EventOrgMemberRemoved,
		catalog.EventOrgRoleChanged,
		catalog.EventDrixyRulesGenerated,
		catalog.EventRuleFileReferencesInvalid,
		catalog.EventIDERulesSynced,
		catalog.EventIDERulesSyncFailed,
		catalog.EventReviewAutoApproved,
		catalog.EventReviewFailed,
		catalog.EventReviewSkippedNoLicense,
		catalog.EventSSODomainVerification,
		catalog.EventRepoReport,
		catalog.EventOrgReport,
		catalog.EventBillingPaymentFailed,
		catalog.EventBillingTrialExpiring,
		catalog.EventByokLlmErrorsThreshold,
		catalog.EventSpendLimitThresholdReached,
		catalog.EventSpendLimitExceededFinal,
	}

	for _, ev := range expectedEvents {
		def, exists := catalog.EventDefaultsMap[ev]
		if !exists {
			t.Fatalf("missing event defaults for %s", ev)
		}
		if def.Category == "" {
			t.Fatalf("event %s has empty category", ev)
		}
		if def.Label == "" {
			t.Fatalf("event %s has empty label", ev)
		}
		if len(def.DefaultChannels) == 0 {
			t.Fatalf("event %s has no default channels", ev)
		}
	}
}

func TestRetryPolicyBackoffAndJitter(t *testing.T) {
	now := time.Now()

	// 1. Default policy: max 5 attempts
	dec := application.DecideRetry(enums.CriticalityTransactional, 1, now)
	if !dec.ShouldRetry {
		t.Fatal("expected retry for attempt 1")
	}
	if dec.MaxAttempts != 5 {
		t.Fatalf("expected 5 max attempts, got %d", dec.MaxAttempts)
	}

	// Attempt 5 -> terminal failure
	decTerminal := application.DecideRetry(enums.CriticalityTransactional, 5, now)
	if decTerminal.ShouldRetry {
		t.Fatal("expected terminal failure on attempt 5")
	}

	// 2. Critical policy: max 8 attempts
	decCrit := application.DecideRetry(enums.CriticalityCritical, 1, now)
	if !decCrit.ShouldRetry {
		t.Fatal("expected retry for critical attempt 1")
	}
	if decCrit.MaxAttempts != 8 {
		t.Fatalf("expected 8 max attempts for critical, got %d", decCrit.MaxAttempts)
	}
}

func TestRoutingRuleConfigGeneration(t *testing.T) {
	engine, _, _ := setupTestEngine()
	cfg := engine.RoutingRules.GetConfig()

	if len(cfg.Events) < 20 {
		t.Fatalf("expected at least 20 events in config, got %d", len(cfg.Events))
	}
	if len(cfg.Channels) != 5 {
		t.Fatalf("expected 5 channels, got %d", len(cfg.Channels))
	}
	if len(cfg.Criticalities) != 4 {
		t.Fatalf("expected 4 criticalities, got %d", len(cfg.Criticalities))
	}
	if len(cfg.Roles) < 5 {
		t.Fatalf("expected roles list to contain standard roles, got %d", len(cfg.Roles))
	}
}

func TestPrAuthorRecipientResolver(t *testing.T) {
	_, _, lookup := setupTestEngine()
	resolver := application.NewPrAuthorRecipientResolver(lookup)
	ctx := context.Background()
	orgID := uuid.New().String()

	// 1. Bot user should be skipped
	botLogin := "dependabot[bot]"
	botEmail := "dependabot@github.com"
	rec, err := resolver.Resolve(ctx, application.PrAuthorRef{Login: &botLogin, Email: &botEmail}, orgID)
	if err != nil || rec != nil {
		t.Fatalf("expected nil recipient for bot user, got %+v", rec)
	}

	// 2. Unregistered user should be skipped
	extEmail := "external@example.com"
	rec, err = resolver.Resolve(ctx, application.PrAuthorRef{Email: &extEmail}, orgID)
	if err != nil || rec != nil {
		t.Fatalf("expected nil recipient for external user, got %+v", rec)
	}

	// 3. Registered user should resolve to KindUser
	devEmail := "dev@scandrix.dev"
	rec, err = resolver.Resolve(ctx, application.PrAuthorRef{Email: &devEmail}, orgID)
	if err != nil || rec == nil {
		t.Fatalf("expected resolved recipient for registered dev, got nil")
	}
	if rec.Kind != recipient.KindUser || rec.UserID == "" {
		t.Fatalf("unexpected recipient shape: %+v", rec)
	}
}

func TestByokErrorCounterThreshold(t *testing.T) {
	engine, emailMock, _ := setupTestEngine()
	ctx := context.Background()
	orgID := uuid.New().String()

	// Record 4 errors -> below threshold (5)
	for i := 0; i < 4; i++ {
		err := engine.ByokCounter.Record(ctx, orgID, "OpenAI", "rate_limit_exceeded")
		if err != nil {
			t.Fatalf("record error failed: %v", err)
		}
	}
	if len(emailMock.Sent) != 0 {
		t.Fatalf("expected no emails sent below threshold, got %d", len(emailMock.Sent))
	}

	// 5th error trips the threshold -> emits byok.llm_errors_threshold
	err := engine.ByokCounter.Record(ctx, orgID, "OpenAI", "rate_limit_exceeded")
	if err != nil {
		t.Fatalf("5th record error failed: %v", err)
	}
	if len(emailMock.Sent) == 0 {
		t.Fatal("expected threshold alert email to be emitted")
	}

	subject := emailMock.Sent[0].Subject
	if !strings.Contains(subject, "BYOK") {
		t.Fatalf("unexpected alert email subject: %s", subject)
	}
}

func TestEndToEndNotificationDispatchAndQuery(t *testing.T) {
	engine, emailMock, lookup := setupTestEngine()
	ctx := context.Background()
	orgUUID := uuid.New()
	orgID := orgUUID.String()

	user1 := lookup.members[0]
	userUUID, _ := uuid.Parse(user1.UserID)

	// 1. Emit a code review auto-approved notification
	err := engine.Service.Emit(
		ctx,
		catalog.EventReviewAutoApproved,
		orgID,
		catalog.ReviewAutoApprovedPayload{
			PRURL:      "https://github.com/scandrix/backend/pull/42",
			RepoName:   "scandrix/backend",
			ApprovedAt: time.Now().UTC().Format(time.RFC3339),
		},
		[]recipient.Recipient{recipient.ByUser(user1.UserID)},
		"test-corr-123",
	)
	if err != nil {
		t.Fatalf("failed to emit review approved event: %v", err)
	}

	// Verify Email delivered
	if len(emailMock.Sent) != 1 {
		t.Fatalf("expected 1 email delivered, got %d", len(emailMock.Sent))
	}
	if !strings.Contains(emailMock.Sent[0].Subject, "Auto-Approved by Drixy") {
		t.Fatalf("unexpected subject: %s", emailMock.Sent[0].Subject)
	}

	// Verify In-App notification stored
	count, err := engine.Query.UnreadCount(ctx, userUUID)
	if err != nil || count != 1 {
		t.Fatalf("expected unread count 1, got %d (err: %v)", count, err)
	}

	list, err := engine.Query.List(ctx, userUUID, 1, 10, false)
	if err != nil || list.Total != 1 {
		t.Fatalf("expected list total 1, got %d", list.Total)
	}
	if list.Data[0].Delivery.Title != "Pull request auto-approved" {
		t.Fatalf("unexpected in-app title: %s", list.Data[0].Delivery.Title)
	}

	// Mark as read
	err = engine.Query.MarkAsRead(ctx, list.Data[0].UUID, userUUID)
	if err != nil {
		t.Fatalf("mark as read failed: %v", err)
	}

	countAfter, _ := engine.Query.UnreadCount(ctx, userUUID)
	if countAfter != 0 {
		t.Fatalf("expected 0 unread after mark read, got %d", countAfter)
	}
}

func TestTemplateRegistryZeroBrandLeaks(t *testing.T) {
	inAppRegistry := channels.NewInAppTemplateRegistry()
	emailRegistry := channels.NewEmailTemplateRegistry("https://app.scandrix.dev")

	testPayload := map[string]interface{}{
		"organizationName": "Acme Corp",
		"token":            "test-token",
		"name":             "Test User",
		"email":            "test@scandrix.dev",
		"repoName":         "scandrix-core",
		"reason":           "check error",
		"rulesCount":       5,
		"domain":           "scandrix.dev",
		"amount":           1999.0,
		"currency":         "usd",
		"daysRemaining":    3,
		"provider":         "Anthropic",
		"errorCount":       10,
		"spentUsd":         45.0,
		"monthlyLimitUsd":  50.0,
		"percentage":       90,
	}

	events := []catalog.Event{
		catalog.EventAuthEmailConfirmation,
		catalog.EventAuthForgotPassword,
		catalog.EventTeamMemberInvited,
		catalog.EventDrixyRulesGenerated,
		catalog.EventSSODomainVerification,
		catalog.EventOrgReport,
		catalog.EventRepoReport,
		catalog.EventReviewAutoApproved,
		catalog.EventReviewFailed,
		catalog.EventReviewSkippedNoLicense,
		catalog.EventBillingPaymentFailed,
		catalog.EventBillingTrialExpiring,
		catalog.EventByokLlmErrorsThreshold,
		catalog.EventSpendLimitThresholdReached,
		catalog.EventSpendLimitExceededFinal,
		catalog.EventRuleFileReferencesInvalid,
	}

	for _, ev := range events {
		// Test In-App template
		inApp := inAppRegistry.ResolveInAppTemplate(ev, testPayload)
		combinedInApp := strings.ToLower(inApp.Title + " " + inApp.Body + " " + inApp.CtaURL)
		if strings.Contains(combinedInApp, "kodus") {
			t.Fatalf("in-app template for %s leaked 'kodus': %s", ev, combinedInApp)
		}
		if strings.Contains(combinedInApp, "kody") {
			t.Fatalf("in-app template for %s leaked 'kody': %s", ev, combinedInApp)
		}

		// Test Email template
		email := emailRegistry.ResolveEmail(ev, testPayload)
		combinedEmail := strings.ToLower(email.Subject + " " + email.From + " " + email.HTML)
		if strings.Contains(combinedEmail, "kodus") {
			t.Fatalf("email template for %s leaked 'kodus': %s", ev, combinedEmail)
		}
		if strings.Contains(combinedEmail, "kody") {
			t.Fatalf("email template for %s leaked 'kody': %s", ev, combinedEmail)
		}
		if strings.Contains(combinedEmail, ".ai") && !strings.Contains(combinedEmail, "openai") {
			t.Fatalf("email template for %s leaked non-.dev domain: %s", ev, combinedEmail)
		}
	}
}
