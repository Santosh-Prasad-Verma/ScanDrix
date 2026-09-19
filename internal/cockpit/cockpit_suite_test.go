package cockpit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cockpit/application"
	appservices "github.com/scandrix/backend/internal/cockpit/application/services"
	"github.com/scandrix/backend/internal/cockpit/application/usecases"
	"github.com/scandrix/backend/internal/cockpit/domain"
	"github.com/scandrix/backend/internal/cockpit/domain/helpers"
	infra "github.com/scandrix/backend/internal/cockpit/infrastructure/services"
	automationdomain "github.com/scandrix/backend/internal/automation/domain"
)

func TestDateRangeUtils(t *testing.T) {
	// 1. Previous period
	prev, err := application.ComputePreviousPeriod("2026-03-01", "2026-03-28")
	if err != nil {
		t.Fatalf("failed to compute previous period: %v", err)
	}
	if prev.EndDate != "2026-02-28" {
		t.Errorf("expected prev end 2026-02-28, got %s", prev.EndDate)
	}
	if prev.StartDate != "2026-02-01" {
		t.Errorf("expected prev start 2026-02-01, got %s", prev.StartDate)
	}

	// 2. Trend calculation
	trendUp := application.ComputeTrend(120, 100, "up")
	if trendUp.Trend != "improved" || trendUp.PercentageChange != 20.0 {
		t.Errorf("expected improved 20%%, got %+v", trendUp)
	}
	trendDown := application.ComputeTrend(80, 100, "down")
	if trendDown.Trend != "improved" || trendDown.PercentageChange != -20.0 {
		t.Errorf("expected improved -20%% for down direction, got %+v", trendDown)
	}

	// 3. Complete weeks
	startW, endW, err := application.LastNCompleteWeeks("2026-03-20", 4)
	if err != nil {
		t.Fatalf("failed complete weeks: %v", err)
	}
	if startW == "" || endW == "" {
		t.Errorf("expected non-empty week dates, got %s, %s", startW, endW)
	}

	// 4. Last N months
	months, err := application.LastNMonths("2026-03-20", 3)
	if err != nil || len(months) != 3 {
		t.Fatalf("expected 3 months, got %d, err: %v", len(months), err)
	}
	if months[2].Label != "Mar" {
		t.Errorf("expected last month Mar, got %s", months[2].Label)
	}
}

func TestDrixyRulesHealthHelper(t *testing.T) {
	// Stale: nil or 0 triggers
	staleState, _ := helpers.ComputeRuleState(nil)
	if staleState != domain.RuleStateStale {
		t.Errorf("expected stale, got %s", staleState)
	}

	// Low data: triggers < 5
	lowDataState, _ := helpers.ComputeRuleState(&domain.DrixyRuleUsageRow{
		RuleID:   "r-1",
		Triggers: 4,
	})
	if lowDataState != domain.RuleStateLowData {
		t.Errorf("expected low_data, got %s", lowDataState)
	}

	// Noisy: triggers >= 5, downvotes >= 3, downvotes > upvotes, downvotes/triggers >= 0.1
	noisyState, _ := helpers.ComputeRuleState(&domain.DrixyRuleUsageRow{
		RuleID:     "r-2",
		Triggers:   10,
		ThumbsDown: 4,
		ThumbsUp:   1,
		Rate:       0.8,
	})
	if noisyState != domain.RuleStateNoisy {
		t.Errorf("expected noisy, got %s", noisyState)
	}

	// Ignored: triggers >= 5, rate <= 0.2
	ignoredState, _ := helpers.ComputeRuleState(&domain.DrixyRuleUsageRow{
		RuleID:     "r-3",
		Triggers:   10,
		ThumbsDown: 1,
		ThumbsUp:   2,
		Rate:       0.15,
	})
	if ignoredState != domain.RuleStateIgnored {
		t.Errorf("expected ignored, got %s", ignoredState)
	}

	// Healthy: triggers >= 5, good rate and sentiment
	healthyState, _ := helpers.ComputeRuleState(&domain.DrixyRuleUsageRow{
		RuleID:     "r-4",
		Triggers:   10,
		ThumbsDown: 0,
		ThumbsUp:   5,
		Rate:       0.7,
	})
	if healthyState != domain.RuleStateHealthy {
		t.Errorf("expected healthy, got %s", healthyState)
	}
}

type mockUserDirectory struct{}

func (m *mockUserDirectory) FindUsers(ctx context.Context, orgID, role, status string) ([]appservices.UserIdentity, error) {
	if role == "OWNER" {
		return []appservices.UserIdentity{
			{UUID: "u-1", Email: "owner@scandrix.dev", Name: "Alice Owner", Role: "OWNER", Status: "ACTIVE"},
		}, nil
	}
	if role == "REPO_ADMIN" {
		return []appservices.UserIdentity{
			{
				UUID:                   "u-2",
				Email:                  "admin@scandrix.dev",
				Name:                   "Bob Admin",
				Role:                   "REPO_ADMIN",
				Status:                 "ACTIVE",
				AssignedRepositoryIDs: []string{"repo-1"},
			},
		}, nil
	}
	return nil, nil
}

func TestCockpitAnalyticsAndReports(t *testing.T) {
	ctx := context.Background()

	reviewSvc := infra.NewInMemoryCockpitReviewAnalyticsService()
	codeHealthSvc := infra.NewInMemoryCockpitCodeHealthService(reviewSvc)
	productivitySvc := infra.NewInMemoryCockpitDeveloperProductivityService()
	reportsSvc := infra.NewCockpitReportsService(reviewSvc, codeHealthSvc, productivitySvc, nil)

	// Seed suggestions
	t1 := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)
	reviewSvc.AddSuggestion(infra.SuggestionRecord{
		ID:                   "sugg-1",
		OrganizationID:       "org-1",
		RepositoryID:         "repo-1",
		RepositoryName:       "scandrix/backend",
		Category:             "security",
		Severity:             "critical",
		ImplementationStatus: "implemented",
		DeliveryStatus:       "sent",
		PRNumber:             10,
		PRStatus:             "closed",
		PRClosedAt:           t1,
		CreatedAt:            t1.Add(-1 * time.Hour),
		ThumbsUp:             3,
	})
	reviewSvc.AddSuggestion(infra.SuggestionRecord{
		ID:                   "sugg-2",
		OrganizationID:       "org-1",
		RepositoryID:         "repo-1",
		RepositoryName:       "scandrix/backend",
		Category:             "performance",
		Severity:             "medium",
		ImplementationStatus: "not_implemented",
		DeliveryStatus:       "sent",
		PRNumber:             10,
		PRStatus:             "closed",
		PRClosedAt:           t1,
		CreatedAt:            t1.Add(-1 * time.Hour),
		BrokenRuleID:         "rule-perf",
		ThumbsDown:           1,
	})

	// Seed executions
	reviewSvc.AddExecution(infra.ReviewExecutionRecord{
		ID:             "exec-1",
		OrganizationID: "org-1",
		RepositoryID:   "repo-1",
		PRNumber:       10,
		Status:         automationdomain.StatusSuccess,
		CreatedAt:      t1,
	})

	// Seed PRs for health and productivity
	codeHealthSvc.AddPullRequest(infra.PullRequestRecord{
		ID:             "pr-1",
		OrganizationID: "org-1",
		RepositoryID:   "repo-1",
		RepositoryName: "scandrix/backend",
		Number:         10,
		Title:          "fix: prevent memory leak",
		IsBugFix:       true,
		Status:         "closed",
		ClosedAt:       t1,
		CreatedAt:      t1.Add(-2 * time.Hour),
	})

	productivitySvc.AddPR(infra.ProductivityPRRecord{
		ID:             "pr-1",
		OrganizationID: "org-1",
		RepositoryID:   "repo-1",
		RepositoryName: "scandrix/backend",
		Number:         10,
		Author:         "drixy-dev",
		LinesAdded:     150,
		LinesDeleted:   50,
		CreatedAt:      t1.Add(-4 * time.Hour),
		FirstCommitAt:  t1.Add(-5 * time.Hour),
		FirstReviewAt:  t1.Add(-2 * time.Hour),
		MergedAt:       t1,
		ClosedAt:       t1,
	})

	q := domain.CockpitRangeQuery{
		OrganizationID: "org-1",
		StartDate:      "2026-03-01",
		EndDate:        "2026-03-28",
	}

	// 1. Implementation rate weekly
	weekly, err := reviewSvc.GetImplementationRateWeekly(ctx, q)
	if err != nil || len(weekly) == 0 {
		t.Fatalf("expected weekly implementation rate, got %d, err: %v", len(weekly), err)
	}
	if weekly[0].Sent != 2 || weekly[0].Implemented != 1 {
		t.Errorf("expected 2 sent, 1 implemented, got sent=%d impl=%d", weekly[0].Sent, weekly[0].Implemented)
	}

	// 2. Implementation rate by severity
	bySev, err := reviewSvc.GetImplementationRateBySeverity(ctx, q)
	if err != nil || len(bySev) == 0 {
		t.Fatalf("expected severity breakdown, err: %v", err)
	}

	// 3. Operational metrics
	ops, err := reviewSvc.GetReviewOperationalMetrics(ctx, q)
	if err != nil || ops.CurrentPeriod.ProcessedReviews != 1 {
		t.Fatalf("expected 1 processed review, got %+v, err: %v", ops, err)
	}

	// 4. Rule groups
	groups, err := reviewSvc.GetReviewQualityByRuleGroup(ctx, q)
	if err != nil || len(groups) != 2 {
		t.Fatalf("expected 2 rule groups, got %d, err: %v", len(groups), err)
	}

	// 5. Build repo section
	section, err := reportsSvc.BuildRepoSection(ctx, "org-1", "scandrix/backend", "2026-03-01", "2026-03-28", nil)
	if err != nil || section == nil {
		t.Fatalf("expected repo section, got nil, err: %v", err)
	}
	if section.Reviews != 1 {
		t.Errorf("expected 1 review, got %d", section.Reviews)
	}
	if section.CriticalImplemented != 1 {
		t.Errorf("expected 1 critical implemented, got %d", section.CriticalImplemented)
	}

	// 6. Build org report
	orgReport, err := reportsSvc.BuildOrgReport(ctx, "org-1", "ScanDrix Corp", "2026-03-01", "2026-03-28")
	if err != nil || orgReport == nil {
		t.Fatalf("expected org report, got nil, err: %v", err)
	}
	if orgReport.Reviews != 1 {
		t.Errorf("expected 1 review in org report, got %d", orgReport.Reviews)
	}

	// 7. Recipients & Send Report Use Cases
	userDir := &mockUserDirectory{}
	recipientsSvc := appservices.NewDefaultReportRecipientsService(userDir, reviewSvc)

	owners, err := recipientsSvc.GetOwners(ctx, "org-1")
	if err != nil || len(owners) != 1 || owners[0].Email != "owner@scandrix.dev" {
		t.Errorf("expected owner@scandrix.dev, got %+v, err: %v", owners, err)
	}

	admins, err := recipientsSvc.GetRepoAdmins(ctx, "org-1")
	if err != nil || len(admins) != 1 || len(admins[0].Repositories) != 1 {
		t.Errorf("expected 1 repo admin with 1 repo, got %+v, err: %v", admins, err)
	}

	sendOrgUC := usecases.NewSendOrgReportUseCase(nil, &mockOrgDirectory{}, recipientsSvc, reportsSvc, nil)
	orgRes, err := sendOrgUC.Execute(ctx, usecases.SendOrgReportInput{
		OrganizationID: "org-1",
		StartDate:      "2026-03-01",
		EndDate:        "2026-03-28",
	})
	if err != nil || orgRes.Sent != 1 {
		t.Errorf("expected 1 sent org report, got %+v, err: %v", orgRes, err)
	}

	sendRepoUC := usecases.NewSendRepoReportUseCase(nil, &mockOrgDirectory{}, recipientsSvc, reportsSvc, nil)
	repoRes, err := sendRepoUC.Execute(ctx, usecases.SendRepoReportInput{
		OrganizationID: "org-1",
		StartDate:      "2026-03-01",
		EndDate:        "2026-03-28",
	})
	if err != nil || repoRes.Sent != 1 {
		t.Errorf("expected 1 sent repo report, got %+v, err: %v", repoRes, err)
	}
}

type mockOrgDirectory struct{}

func (m *mockOrgDirectory) GetOrganizationName(ctx context.Context, orgID string) (string, error) {
	if orgID == "org-1" {
		return "ScanDrix Corp", nil
	}
	return "", fmt.Errorf("org not found")
}

type mockPRCounter struct {
	count int
	err   error
}

func (m *mockPRCounter) CountPullRequests(ctx context.Context, orgID string, limit int) (int, error) {
	return m.count, m.err
}

func TestCockpitTierPolicyValidationAndHealth(t *testing.T) {
	ctx := context.Background()

	// 1. Tier Policy
	if domain.IsCockpitTierAllowed(nil) {
		t.Error("expected nil license to be disallowed")
	}
	if domain.IsCockpitTierAllowed(&domain.OrganizationLicenseStatus{IsValid: false}) {
		t.Error("expected invalid license to be disallowed")
	}
	if domain.IsCockpitTierAllowed(&domain.OrganizationLicenseStatus{IsValid: true, Plan: "free_byok", SubscriptionStatus: "active"}) {
		t.Error("expected free_byok to be disallowed")
	}
	if domain.IsCockpitTierAllowed(&domain.OrganizationLicenseStatus{IsValid: true, Plan: "teams", SubscriptionStatus: "canceled"}) {
		t.Error("expected canceled license to be disallowed")
	}
	if !domain.IsCockpitTierAllowed(&domain.OrganizationLicenseStatus{IsValid: true, Plan: "teams", SubscriptionStatus: "active"}) {
		t.Error("expected active teams cloud to be allowed")
	}
	if !domain.IsCockpitTierAllowed(&domain.OrganizationLicenseStatus{IsValid: true, Plan: "enterprise", SubscriptionStatus: "active"}) {
		t.Error("expected active enterprise cloud to be allowed")
	}
	if !domain.IsCockpitTierAllowed(&domain.OrganizationLicenseStatus{IsValid: true, Plan: "enterprise", SubscriptionStatus: "trial"}) {
		t.Error("expected trial to be allowed")
	}
	if !domain.IsCockpitTierAllowed(&domain.OrganizationLicenseStatus{IsValid: true, Plan: "enterprise", SubscriptionStatus: "active", IsSelfHosted: true}) {
		t.Error("expected enterprise self-hosted with active status to be allowed")
	}

	// 2. Source Resolver
	resolver := infra.NewCockpitSourceResolver()
	src, err := resolver.Resolve(ctx, "org-1")
	if err != nil || src != domain.CockpitSourceInternal {
		t.Errorf("expected internal source, got %s, err: %v", src, err)
	}

	// 3. Validation Service
	valSvc := infra.NewCockpitValidationService(&mockPRCounter{count: 15})
	val, err := valSvc.Validate(ctx, "org-1")
	if err != nil || !val.HasData || val.PullRequestsCount != 15 {
		t.Errorf("expected validation hasData=true, count=15, got %+v, err: %v", val, err)
	}
	emptyValSvc := infra.NewCockpitValidationService(&mockPRCounter{count: 0})
	valEmpty, err := emptyValSvc.Validate(ctx, "org-1")
	if err != nil || valEmpty.HasData || valEmpty.PullRequestsCount != 0 {
		t.Errorf("expected validation hasData=false, count=0, got %+v, err: %v", valEmpty, err)
	}

	// 4. Health Service
	healthSvc := infra.NewCockpitHealthService(nil)
	h := healthSvc.Ping(ctx)
	if h.Status != "ok" || !h.AnalyticsPostgres.Reachable {
		t.Errorf("expected health ok, got %+v", h)
	}

	now := time.Now().UTC()
	start := now.Add(-2 * time.Hour)
	fin := now.Add(-1 * time.Hour)
	healthSvc.RecordRun(infra.IngestionRunSummary{
		ID:                  "run-1",
		Source:              "pull_requests",
		Status:              "ok",
		StartedAt:           &start,
		FinishedAt:          &fin,
		Scanned:             50,
		PRsUpserted:         45,
		SuggestionsInserted: 120,
	})

	runHealth := healthSvc.RunsSummary(ctx, "pull_requests")
	if runHealth.Last == nil || runHealth.Last.ID != "run-1" {
		t.Errorf("expected last run-1, got %+v", runHealth.Last)
	}
	if runHealth.LagHours == nil || *runHealth.LagHours < 0.5 {
		t.Errorf("expected lag around 1 hour, got %+v", runHealth.LagHours)
	}
}

