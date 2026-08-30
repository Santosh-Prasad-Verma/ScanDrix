package issues_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/issues"
	"github.com/scandrix/backend/pkg/models"
)

func TestIssueServiceLifecycleAndAutoCreation(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()
	repoID := uuid.New()
	reviewID := uuid.New()

	store := issues.NewMemoryIssueStore()
	policy := issues.DefaultIssueCreationPolicy() // Threshold = High
	svc := issues.NewIssueService(store, policy)

	findings := []models.CodeFinding{
		{
			ID:          uuid.New(),
			FilePath:    "auth/jwt.go",
			StartLine:   10,
			EndLine:     15,
			Severity:    models.SeverityCritical,
			Category:    "SECURITY",
			Title:       "Hardcoded JWT Secret",
			Description: "JWT secret is hardcoded in source code",
			Remediation: "Load secret from env",
			Fingerprint: "fp-cwe-798-jwt",
		},
		{
			ID:          uuid.New(),
			FilePath:    "utils/logger.go",
			StartLine:   5,
			EndLine:     6,
			Severity:    models.SeverityLow,
			Category:    "STYLE",
			Title:       "Missing docstring",
			Description: "Public function lacks comment",
			Fingerprint: "fp-style-missing-doc",
		},
	}

	// 1. Auto-create from findings (Low should be skipped, Critical should be created)
	created, err := svc.AutoCreateFromFindings(ctx, wsID, repoID, reviewID, findings)
	if err != nil {
		t.Fatalf("failed auto-creating issues: %v", err)
	}

	if len(created) != 1 {
		t.Fatalf("expected 1 issue created (only Critical), got %d", len(created))
	}
	if created[0].Severity != models.SeverityCritical || created[0].Status != issues.StatusOpen {
		t.Errorf("unexpected issue attributes: %+v", created[0])
	}

	// 2. Query open issues
	openIssues, err := svc.ListIssues(ctx, wsID, issues.StatusOpen)
	if err != nil || len(openIssues) != 1 {
		t.Fatalf("expected 1 open issue, got %d (err: %v)", len(openIssues), err)
	}

	// 3. Update status to IN_PROGRESS
	issueID := openIssues[0].ID
	err = svc.UpdateStatus(ctx, wsID, issueID, issues.StatusInProgress)
	if err != nil {
		t.Fatalf("failed updating issue status: %v", err)
	}

	// 4. Resolve on merge using fingerprint
	resolvedCount, err := svc.ResolveIssuesOnMerge(ctx, wsID, []string{"fp-cwe-798-jwt"})
	if err != nil {
		t.Fatalf("failed resolving issues on merge: %v", err)
	}
	if resolvedCount != 1 {
		t.Fatalf("expected 1 issue resolved, got %d", resolvedCount)
	}

	// 5. Verify status is RESOLVED and has resolved timestamp
	resolvedIssues, err := svc.ListIssues(ctx, wsID, issues.StatusResolved)
	if err != nil || len(resolvedIssues) != 1 {
		t.Fatalf("expected 1 resolved issue, got %d (err: %v)", len(resolvedIssues), err)
	}
	if resolvedIssues[0].ResolvedAt == nil {
		t.Error("expected ResolvedAt timestamp to be set")
	}
}
