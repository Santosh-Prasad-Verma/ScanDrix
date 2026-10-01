package database

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

// TestBatchInsertFindingsUnderRuntimeRole verifies the RLS conversion of
// BatchInsertFindings against a real database using a non-superuser role.
//
// This path could not be covered by the end-to-end review run: with a read-only
// SCM token and a rate-limited LLM, the review legitimately produced zero
// findings, so the insert was never exercised. A unit test with a mocked
// client would prove nothing, because the failure mode this guards against is
// the policy silently matching zero rows or rejecting the write outright.
//
// Skips unless SCANDRIX_E2E_RUNTIME_DSN points at a live database.
func TestBatchInsertFindingsUnderRuntimeRole(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to a live database to run this test")
	}

	ctx := context.Background()
	client, err := NewClient(ctx, dsn)
	if err != nil {
		t.Fatalf("connect as runtime role: %v", err)
	}
	defer client.Close()

	// The role must actually be least-privilege, otherwise this test proves
	// nothing: a superuser bypasses RLS and every write would succeed.
	var isSuper, bypassRLS bool
	if err := client.Pool.QueryRow(ctx,
		"SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user",
	).Scan(&isSuper, &bypassRLS); err != nil {
		t.Fatalf("read role attributes: %v", err)
	}
	if isSuper || bypassRLS {
		t.Fatalf("connected as a privileged role (superuser=%v bypassrls=%v); this test would prove nothing", isSuper, bypassRLS)
	}

	repo := NewRepository(client)

	// Use a real workspace so the foreign keys on review_id and workspace_id hold.
	wsID, reviewID, repoID := seedReview(t, ctx, repo)

	findings := []models.CodeFinding{
		{
			ReviewID: reviewID, WorkspaceID: wsID, FilePath: "main.go",
			StartLine: 5, EndLine: 7, Severity: models.SeverityHigh,
			Category: "SECURITY", Title: "Hardcoded credential",
			Description: "a password is assigned to a literal", Fingerprint: "fp-rls-1",
		},
		{
			ReviewID: reviewID, WorkspaceID: wsID, FilePath: "main.go",
			StartLine: 6, EndLine: 6, Severity: models.SeverityMedium,
			Category: "STYLE", Title: "Ignored error return",
			Description: "error discarded", Fingerprint: "fp-rls-2",
		},
	}

	if err := repo.BatchInsertFindings(ctx, wsID, findings); err != nil {
		t.Fatalf("BatchInsertFindings under least privilege: %v", err)
	}

	// The count MUST run inside the tenant context. A raw pool query returns
	// zero rows under RLS for a non-superuser, which reads as "the insert wrote
	// nothing" even when it succeeded. That is precisely the silent failure
	// mode these conversions exist to prevent.
	var count int
	if err := client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT count(*) FROM code_findings WHERE review_id = $1", reviewID,
		).Scan(&count)
	}); err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if count != len(findings) {
		t.Fatalf("expected %d findings persisted, got %d", len(findings), count)
	}

	// A different tenant must not see them, which is what the conversion buys.
	var leaked int
	other := uuid.New()
	if err := client.ExecWithTenant(ctx, other, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT count(*) FROM code_findings WHERE review_id = $1", reviewID,
		).Scan(&leaked)
	}); err != nil {
		t.Fatalf("isolation check: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("findings leaked to a different workspace: %d rows", leaked)
	}

	cleanup(t, ctx, client, wsID, reviewID, repoID)
}

// seedReview creates the workspace, repository and review rows the findings
// foreign keys require, then returns their ids.
func seedReview(t *testing.T, ctx context.Context, repo *Repository) (wsID, reviewID, repoID uuid.UUID) {
	t.Helper()
	wsID, reviewID, repoID = uuid.New(), uuid.New(), uuid.New()

	ws := &models.Workspace{ID: wsID, Slug: "rls-findings-" + wsID.String()[:8], Name: "RLS Findings", Status: "ACTIVE"}
	if err := repo.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := repo.TrackRepository(ctx, wsID, models.SCMProvider("github"), "rls/repo", "rls/repo", "main"); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	tracked, err := repo.GetTrackedRepositoryByNamespace(ctx, models.SCMProvider("github"), "rls/repo")
	if err != nil {
		t.Fatalf("read back repository: %v", err)
	}
	repoID = tracked.ID

	if err := repo.CreateReview(ctx, &models.PullRequestReview{
		ID: reviewID, WorkspaceID: wsID, RepositoryID: repoID,
		PullNumber: 1, Title: "rls findings", HeadSHA: "abc", BaseSHA: "def",
		State: models.ReviewStateProcessing,
	}); err != nil {
		t.Fatalf("seed review: %v", err)
	}
	return wsID, reviewID, repoID
}

func cleanup(t *testing.T, ctx context.Context, client *Client, wsID, reviewID, repoID uuid.UUID) {
	t.Helper()
	// Deleting the workspace cascades to the review, findings and repository.
	_, _ = client.Pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", wsID)
	_ = reviewID
	_ = repoID
}
