package dashboard_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/usecases/dashboard"
)

func TestDashboardAggregationAndFacets(t *testing.T) {
	agg := dashboard.NewDashboardAggregator()
	wsID := uuid.New()
	repoID := uuid.New()
	now := time.Now().UTC()

	// 1. Ingest PR 1: Reviewed today with critical security finding
	agg.IngestRecord(dashboard.PullRequestRecord{
		PRID:         uuid.New(),
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		RepoName:     "scandrix/core",
		PRNumber:     101,
		Title:        "Add auth credentials",
		URL:          "https://github.com/scandrix/core/pull/101",
		Author:       "alice",
		Status:       "reviewed",
		SeverityMax:  "critical",
		FindingCount: 2,
		OpenedAt:     now.Add(-2 * time.Hour),
		ReviewedAt:   &now,
	})

	// 2. Ingest PR 2: Reviewed today, clean approved
	agg.IngestRecord(dashboard.PullRequestRecord{
		PRID:         uuid.New(),
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		RepoName:     "scandrix/core",
		PRNumber:     102,
		Title:        "Refactor logging",
		URL:          "https://github.com/scandrix/core/pull/102",
		Author:       "bob",
		Status:       "reviewed",
		SeverityMax:  "clean",
		FindingCount: 0,
		OpenedAt:     now.Add(-1 * time.Hour),
		ReviewedAt:   &now,
	})

	// 3. Ingest PR 3: Errored review today
	agg.IngestRecord(dashboard.PullRequestRecord{
		PRID:         uuid.New(),
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		RepoName:     "scandrix/infra",
		PRNumber:     45,
		Title:        "Terraform updates",
		URL:          "https://github.com/scandrix/infra/pull/45",
		Author:       "charlie",
		Status:       "errored",
		OpenedAt:     now.Add(-30 * time.Minute),
		ReviewedAt:   &now,
	})

	// 4. Ingest PR 4: Awaiting review opened today
	agg.IngestRecord(dashboard.PullRequestRecord{
		PRID:         uuid.New(),
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		RepoName:     "scandrix/core",
		PRNumber:     103,
		Title:        "Feature X",
		URL:          "https://github.com/scandrix/core/pull/103",
		Author:       "alice",
		Status:       "awaiting",
		OpenedAt:     now.Add(-5 * time.Minute),
	})

	// --- VERIFY DAILY DIGEST ---
	digest := agg.GetDailyDigest(wsID)
	if digest.ReviewedToday != 2 {
		t.Fatalf("expected 2 reviewed today, got %d", digest.ReviewedToday)
	}
	if digest.NeedsAttention != 1 {
		t.Fatalf("expected 1 needs attention, got %d", digest.NeedsAttention)
	}
	if digest.ErroredToday != 1 {
		t.Fatalf("expected 1 errored today, got %d", digest.ErroredToday)
	}
	if digest.AwaitingReview != 1 {
		t.Fatalf("expected 1 awaiting review, got %d", digest.AwaitingReview)
	}

	// --- VERIFY AWAITING PULL REQUESTS ---
	awaitingList := agg.GetAwaitingPullRequests(wsID)
	if len(awaitingList) != 1 {
		t.Fatalf("expected 1 awaiting PR, got %d", len(awaitingList))
	}
	if awaitingList[0].PRNumber != 103 || awaitingList[0].Author != "alice" {
		t.Fatalf("unexpected awaiting PR details: %+v", awaitingList[0])
	}

	// --- VERIFY FACETS ---
	facets := agg.GetFacets(wsID)
	if facets.TotalReviewed != 2 {
		t.Fatalf("expected total reviewed 2, got %d", facets.TotalReviewed)
	}
	if facets.CleanApproved != 1 {
		t.Fatalf("expected clean approved 1, got %d", facets.CleanApproved)
	}
	if facets.NeedsAttention != 1 {
		t.Fatalf("expected needs attention 1, got %d", facets.NeedsAttention)
	}
	if facets.SeverityCounts["critical"] != 1 {
		t.Fatalf("expected 1 critical severity count, got %d", facets.SeverityCounts["critical"])
	}
	if facets.TopAuthors["alice"] != 2 { // PR 101 and PR 103
		t.Fatalf("expected alice count 2, got %d", facets.TopAuthors["alice"])
	}
	if facets.RepositoryStats["scandrix/core"] != 3 {
		t.Fatalf("expected scandrix/core count 3, got %d", facets.RepositoryStats["scandrix/core"])
	}
}
