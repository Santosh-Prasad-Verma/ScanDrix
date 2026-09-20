// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: repositories_test
// File: repositories_test.go
// ═══════════════════════════════════════════════════════════════

package repositories_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/scandrix/backend/internal/platformdata/domain/contracts"
	"github.com/scandrix/backend/internal/platformdata/domain/enums"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
	"github.com/scandrix/backend/internal/platformdata/infrastructure/repositories"
)

func TestMemoryPullRequestsRepository_CompleteLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMemoryPullRequestsRepository()

	orgID := uuid.NewString()
	repoID := uuid.NewString()

	pr := &models.PullRequest{
		Title:          "Refactor authentication middleware",
		Status:         "OPEN",
		Number:         42,
		URL:            "https://github.com/scandrix/backend/pull/42",
		BaseBranchRef:  "main",
		HeadBranchRef:  "feat/auth-refactor",
		Provider:       "github",
		OrganizationID: orgID,
		Repository: models.RepositoryInfo{
			ID:       repoID,
			Name:     "backend",
			FullName: "scandrix/backend",
		},
		User: models.PullRequestUser{
			ID:       "usr-1",
			Username: "drixy-dev",
			Name:     "Drixy Developer",
			Email:    "dev@scandrix.dev",
		},
		Files: []models.File{
			{
				ID:       "f-1",
				Path:     "auth/middleware.go",
				Filename: "middleware.go",
				Status:   "modified",
				Added:    15,
				Deleted:  5,
				Changes:  20,
				Suggestions: []models.Suggestion{
					{
						ID:                   "sug-1",
						RelevantFile:         "auth/middleware.go",
						Language:             "go",
						SuggestionContent:    "Validate token expiry before cryptographic verification",
						Severity:             "CRITICAL",
						DeliveryStatus:       enums.DeliveryStatusSent,
						ImplementationStatus: enums.ImplementationStatusNotImplemented,
						PriorityStatus:       enums.PriorityStatusPrioritized,
					},
					{
						ID:                   "sug-2",
						RelevantFile:         "auth/middleware.go",
						Language:             "go",
						SuggestionContent:    "Add telemetry span attribute for tenant ID",
						Severity:             "LOW",
						DeliveryStatus:       enums.DeliveryStatusSent,
						ImplementationStatus: enums.ImplementationStatusImplemented,
						PriorityStatus:       enums.PriorityStatusPrioritized,
					},
				},
			},
		},
	}

	// 1. Create PR
	saved, err := repo.Create(ctx, pr)
	if err != nil {
		t.Fatalf("failed creating PR: %v", err)
	}
	if saved.UUID == "" {
		t.Errorf("expected non-empty UUID on saved PR")
	}

	// 2. FindByID
	found, err := repo.FindByID(ctx, saved.UUID)
	if err != nil || found == nil {
		t.Fatalf("failed finding PR by ID: %v", err)
	}
	if found.Title != pr.Title {
		t.Errorf("expected title %s, got %s", pr.Title, found.Title)
	}

	// 3. FindByNumberAndRepositoryID & FindOne
	foundOne, err := repo.FindOne(ctx, orgID, repoID, 42)
	if err != nil || foundOne == nil {
		t.Fatalf("failed FindOne: %v", err)
	}
	if foundOne.Number != 42 {
		t.Errorf("expected PR number 42, got %d", foundOne.Number)
	}

	// 4. Find (pagination)
	list, err := repo.Find(ctx, orgID, repoID, 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 PR in list, got %d", len(list))
	}

	// 5. FindPRNumbersByTitleAndOrganization
	byTitle, err := repo.FindPRNumbersByTitleAndOrganization(ctx, "authentication", orgID, []string{repoID})
	if err != nil || len(byTitle) != 1 {
		t.Fatalf("expected 1 match by title, got %d (err: %v)", len(byTitle), err)
	}
	if byTitle[0].Number != 42 {
		t.Errorf("expected matched number 42, got %d", byTitle[0].Number)
	}

	// 6. FindManyByNumbersAndRepositoryIDs
	many, err := repo.FindManyByNumbersAndRepositoryIDs(ctx, []struct {
		Number       int
		RepositoryID string
	}{
		{Number: 42, RepositoryID: repoID},
	}, orgID)
	if err != nil || len(many) != 1 {
		t.Fatalf("expected 1 PR from FindManyByNumbersAndRepositoryIDs, got %d", len(many))
	}

	// 7. FindManyByNumbers
	userMappings, err := repo.FindManyByNumbers(ctx, []int{42}, orgID)
	if err != nil || len(userMappings) != 1 {
		t.Fatalf("expected 1 user mapping, got %d", len(userMappings))
	}
	if userMappings[0].User.Username != "drixy-dev" {
		t.Errorf("expected username drixy-dev, got %s", userMappings[0].User.Username)
	}

	// 8. FindNumbersByRepositoryID
	nums, err := repo.FindNumbersByRepositoryID(ctx, orgID, repoID, nil)
	if err != nil || len(nums) != 1 || nums[0] != 42 {
		t.Fatalf("expected [42], got %v", nums)
	}

	// 9. FindSuggestionCountsByNumbersAndRepositoryIds
	counts, err := repo.FindSuggestionCountsByNumbersAndRepositoryIds(ctx, []struct {
		Number       int
		RepositoryID string
	}{
		{Number: 42, RepositoryID: repoID},
	}, orgID)
	if err != nil {
		t.Fatalf("failed fetching suggestion counts: %v", err)
	}
	key := repoID + "_42"
	c, ok := counts[key]
	if !ok {
		t.Fatalf("expected entry for %s in counts map", key)
	}
	if c.Sent != 2 {
		t.Errorf("expected 2 sent suggestions, got %d", c.Sent)
	}
	if c.BySeverity.Critical != 1 {
		t.Errorf("expected 1 critical suggestion, got %d", c.BySeverity.Critical)
	}
	if c.BySeverity.Low != 1 {
		t.Errorf("expected 1 low suggestion, got %d", c.BySeverity.Low)
	}

	// 10. FindOpenPullRequestKeysOpenedSince
	openedKeys, err := repo.FindOpenPullRequestKeysOpenedSince(ctx, time.Now().Add(-1*time.Hour), orgID, []string{repoID})
	if err != nil || len(openedKeys) != 1 {
		t.Fatalf("expected 1 opened key, got %d", len(openedKeys))
	}

	// 11. FindDistinctAuthorsByRepositoryIds
	authors, err := repo.FindDistinctAuthorsByRepositoryIds(ctx, orgID, []string{repoID}, "drixy", 10)
	if err != nil || len(authors) != 1 {
		t.Fatalf("expected 1 author suggestion, got %d", len(authors))
	}
	if authors[0].Username != "drixy-dev" || authors[0].Count != 1 {
		t.Errorf("unexpected author data: %+v", authors[0])
	}

	// 12. CountDeliveredPullRequests
	deliveredCount, err := repo.CountDeliveredPullRequests(ctx, orgID, []string{repoID}, contracts.DeliveredFilterOpts{
		OpenOnly:       true,
		UnresolvedOnly: true,
		Severities:     []string{"critical"},
	})
	if err != nil {
		t.Fatalf("CountDeliveredPullRequests failed: %v", err)
	}
	if deliveredCount != 1 {
		t.Errorf("expected 1 delivered PR matching unresolved critical filter, got %d", deliveredCount)
	}

	// 13. FindFileWithSuggestions
	file, err := repo.FindFileWithSuggestions(ctx, orgID, repoID, 42, "auth/middleware.go")
	if err != nil || file == nil {
		t.Fatalf("failed finding file with suggestions: %v", err)
	}
	if len(file.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions in file, got %d", len(file.Suggestions))
	}

	// 14. FindSuggestionsByPR
	suggestions, err := repo.FindSuggestionsByPR(ctx, orgID, repoID, 42, enums.DeliveryStatusSent)
	if err != nil || len(suggestions) != 2 {
		t.Fatalf("expected 2 delivered suggestions, got %d", len(suggestions))
	}

	// 15. BulkApplyFileChanges
	bulkRes, err := repo.BulkApplyFileChanges(ctx, saved.UUID, orgID, []models.FileBulkOp{
		{
			Kind: "addFile",
			File: &models.File{
				Path:     "auth/telemetry.go",
				Filename: "telemetry.go",
				Added:    10,
				Deleted:  2,
				Changes:  12,
			},
		},
		{
			Kind: "addSuggestions",
			Suggestions: []models.Suggestion{
				{
					RelevantFile:      "auth/telemetry.go",
					SuggestionContent: "Add high cardinality tag check",
					Severity:          "MEDIUM",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BulkApplyFileChanges failed: %v", err)
	}
	if bulkRes.Modified == 0 {
		t.Errorf("expected modified entries from bulk apply, got %d", bulkRes.Modified)
	}

	// 16. ComputeFileTotals
	added, deleted, changes, err := repo.ComputeFileTotals(ctx, saved.UUID, orgID)
	if err != nil {
		t.Fatalf("ComputeFileTotals failed: %v", err)
	}
	if added != 25 || deleted != 7 || changes != 32 {
		t.Errorf("unexpected file totals: added=%d deleted=%d changes=%d (expected 25, 7, 32)", added, deleted, changes)
	}

	// 17. UpdateSuggestion
	err = repo.UpdateSuggestion(ctx, orgID, "sug-1", map[string]interface{}{
		"implementation_status": string(enums.ImplementationStatusImplemented),
	})
	if err != nil {
		t.Fatalf("UpdateSuggestion failed: %v", err)
	}
	updatedPR, _ := repo.FindByID(ctx, saved.UUID)
	if updatedPR.Files[0].Suggestions[0].ImplementationStatus != enums.ImplementationStatusImplemented {
		t.Errorf("expected updated implementation status, got %s", updatedPR.Files[0].Suggestions[0].ImplementationStatus)
	}

	// 18. Flags update
	err = repo.UpdateSyncedSuggestionsFlag(ctx, []int{42}, repoID, orgID, true)
	if err != nil {
		t.Fatalf("UpdateSyncedSuggestionsFlag failed: %v", err)
	}
	err = repo.UpdateSyncedWithIssuesFlag(ctx, 42, repoID, orgID, true)
	if err != nil {
		t.Fatalf("UpdateSyncedWithIssuesFlag failed: %v", err)
	}

	finalPR, _ := repo.FindByID(ctx, saved.UUID)
	if !finalPR.SyncedEmbeddedSuggestions || !finalPR.SyncedWithIssues {
		t.Errorf("expected synced flags to be true")
	}
}

func TestPostgresPullRequestsRepository_Live(t *testing.T) {
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load("../../../../.env")

	dbURL := os.Getenv("DIRECT_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("SUPABASE_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("PostgreSQL DATABASE_URL not set; skipping live DB test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Failed to initialize pgxpool (%v); skipping live DB test", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL unreachable (%v); skipping live DB test", err)
	}

	repo := repositories.NewPostgresPullRequestsRepository(pool)
	orgID := uuid.New()
	repoID := uuid.NewString()

	// Insert workspace to satisfy foreign key constraint
	_, err = pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, name, status)
		VALUES ($1, $2, 'Platform Data Test Corp', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING
	`, orgID, "platform-test-"+orgID.String()[:8])
	if err != nil {
		t.Fatalf("failed inserting test workspace: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM platform_pull_requests WHERE workspace_id = $1", orgID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM workspaces WHERE id = $1", orgID)
	}()

	pr := &models.PullRequest{
		Title:          "Postgres Live PR Test",
		Status:         "OPEN",
		Number:         101,
		URL:            "https://github.com/scandrix/backend/pull/101",
		BaseBranchRef:  "main",
		HeadBranchRef:  "feat/pg-test",
		Provider:       "github",
		OrganizationID: orgID.String(),
		Repository: models.RepositoryInfo{
			ID:       repoID,
			Name:     "backend",
			FullName: "scandrix/backend",
		},
		User: models.PullRequestUser{
			ID:       "usr-pg",
			Username: "pg-tester",
			Name:     "Postgres Tester",
			Email:    "tester@scandrix.dev",
		},
		Files: []models.File{
			{
				ID:       "f-pg-1",
				Path:     "db/query.go",
				Filename: "query.go",
				Status:   "modified",
				Added:    5,
				Deleted:  1,
				Changes:  6,
				Suggestions: []models.Suggestion{
					{
						ID:                   "sug-pg-1",
						RelevantFile:         "db/query.go",
						Language:             "go",
						SuggestionContent:    "Use parameterized queries for security",
						Severity:             "HIGH",
						DeliveryStatus:       enums.DeliveryStatusSent,
						ImplementationStatus: enums.ImplementationStatusNotImplemented,
						PriorityStatus:       enums.PriorityStatusPrioritized,
					},
				},
			},
		},
	}

	saved, err := repo.Create(ctx, pr)
	if err != nil {
		t.Fatalf("Live Postgres Create failed: %v", err)
	}
	if saved.UUID == "" {
		t.Errorf("Expected saved PR to have non-empty UUID")
	}

	found, err := repo.FindByID(ctx, saved.UUID)
	if err != nil || found == nil {
		t.Fatalf("Live Postgres FindByID failed: %v", err)
	}
	if found.Title != pr.Title {
		t.Errorf("expected title %s, got %s", pr.Title, found.Title)
	}

	counts, err := repo.FindSuggestionCountsByNumbersAndRepositoryIds(ctx, []struct {
		Number       int
		RepositoryID string
	}{
		{Number: 101, RepositoryID: repoID},
	}, orgID.String())
	if err != nil {
		t.Fatalf("Live Postgres FindSuggestionCountsByNumbersAndRepositoryIds failed: %v", err)
	}
	key := repoID + "_101"
	if c, ok := counts[key]; ok {
		if c.Sent != 1 {
			t.Errorf("expected 1 sent suggestion in live DB, got %d", c.Sent)
		}
	} else {
		t.Errorf("key %s not found in counts map: %+v", key, counts)
	}
}
