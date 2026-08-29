package cockpit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cockpit"
)

func TestCockpitAggregatorAndReport(t *testing.T) {
	agg := cockpit.NewCockpitAggregator()

	// 1. Debt calculation
	debt := agg.CalculateSecurityDebt(3, 5, 15.0) // (3*5 + 5*2) * (1 + 15/30) = 25 * 1.5 = 37.5
	if debt != 37.5 {
		t.Fatalf("expected security debt 37.5, got %f", debt)
	}

	// 2. Executive report markdown formatting
	wsID := uuid.New()
	report := cockpit.ExecutiveCockpitReport{
		WorkspaceID:  wsID,
		ReportPeriod: "2026-W34",
		Health: cockpit.CodeHealthMetrics{
			WorkspaceID:          wsID,
			OpenVulnerabilities:  8,
			CriticalExposureDays: 4.2,
			SecurityDebtIndex:    debt,
			MeanTimeToRemediate:  6.5,
		},
		Productivity: cockpit.DeveloperProductivityMetrics{
			WorkspaceID:            wsID,
			PullRequestsReviewed:   42,
			AverageTurnaroundHours: 1.2,
			AutomatedReviewRatio:   0.85,
			ReviewerSaturationRate: 0.45,
		},
		GeneratedAt: time.Now().UTC(),
	}

	md := report.FormatMarkdown()
	if !strings.Contains(md, "Executive Engineering Cockpit Report") {
		t.Fatal("expected report markdown header")
	}
	if !strings.Contains(md, "**Security Debt Index:** 37.50") {
		t.Fatal("expected debt index in markdown")
	}
	if !strings.Contains(md, "**Automated Review Ratio:** 85.0%") {
		t.Fatal("expected automated review ratio in markdown")
	}
}
