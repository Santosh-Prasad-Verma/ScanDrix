package dora_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/dora"
)

func TestDORAMetricsCalculationAndStore(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	now := time.Now().UTC()
	start := now.Add(-5 * 24 * time.Hour) // 5 day window
	end := now

	store := dora.NewDORAStore()

	// 1. Record Elite Deployments: 10 deploys in 5 days (2/day) with 30m lead time
	for i := 0; i < 10; i++ {
		err := store.RecordDeployment(ctx, dora.DeploymentRecord{
			WorkspaceID:   wsID,
			RepoNamespace: "acme/api",
			CommitSHA:     "sha123",
			DeployedAt:    start.Add(time.Duration(i*12) * time.Hour),
			LeadDuration:  30 * time.Minute,
			IsFailed:      false,
		})
		if err != nil {
			t.Fatalf("record deploy failed: %v", err)
		}
	}

	reportElite, err := store.GenerateReport(ctx, wsID, start, end, 25, 40)
	if err != nil {
		t.Fatalf("generate report failed: %v", err)
	}

	if reportElite.OverallTier != dora.TierElite {
		t.Fatalf("expected ELITE tier, got %s", reportElite.OverallTier)
	}
	if reportElite.DeploymentFrequency.Tier != dora.TierElite || reportElite.DeploymentFrequency.DeploysPerDay < 1.9 {
		t.Fatalf("unexpected deploy freq: %+v", reportElite.DeploymentFrequency)
	}
	if reportElite.LeadTimeForChanges.Tier != dora.TierElite {
		t.Fatalf("unexpected lead time tier: %+v", reportElite.LeadTimeForChanges)
	}
	if reportElite.ChangeFailureRate.FailureRate != 0.0 || reportElite.ChangeFailureRate.Tier != dora.TierElite {
		t.Fatalf("unexpected failure rate: %+v", reportElite.ChangeFailureRate)
	}
	if reportElite.ReviewVelocity.EngineeringHoursSaved != 20.0 { // 40 defects * 0.5 = 20.0 hrs
		t.Fatalf("expected 20 hours saved, got %f", reportElite.ReviewVelocity.EngineeringHoursSaved)
	}

	// 2. Add an incident and test MTTR calculation
	failedID := uuid.New()
	failedDeployTime := now.Add(-2 * time.Hour)
	err = store.RecordDeployment(ctx, dora.DeploymentRecord{
		ID:            failedID,
		WorkspaceID:   wsID,
		RepoNamespace: "acme/api",
		CommitSHA:     "bad_sha",
		DeployedAt:    failedDeployTime,
		LeadDuration:  45 * time.Minute,
		IsFailed:      true,
	})
	if err != nil {
		t.Fatalf("record failed deploy failed: %v", err)
	}

	// Resolve incident 45 minutes later
	resolvedTime := failedDeployTime.Add(45 * time.Minute)
	if err := store.ResolveIncident(ctx, failedID, resolvedTime); err != nil {
		t.Fatalf("resolve incident failed: %v", err)
	}

	reportWithIncident, err := store.GenerateReport(ctx, wsID, start, end, 25, 40)
	if err != nil {
		t.Fatalf("generate report with incident failed: %v", err)
	}

	if reportWithIncident.TimeToRestore.IncidentCount != 1 {
		t.Fatalf("expected 1 incident, got %d", reportWithIncident.TimeToRestore.IncidentCount)
	}
	if reportWithIncident.TimeToRestore.AverageMTTR != 45*time.Minute {
		t.Fatalf("expected 45m MTTR, got %v", reportWithIncident.TimeToRestore.AverageMTTR)
	}
	if reportWithIncident.TimeToRestore.Tier != dora.TierElite {
		t.Fatalf("expected ELITE MTTR for <1h, got %s", reportWithIncident.TimeToRestore.Tier)
	}
}
