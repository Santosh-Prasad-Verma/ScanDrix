package review_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
)

type mockSCMPublisher struct {
	submittedReviews []github.PullReviewSubmission
	updatedCheckRuns []github.UpdateCheckRunRequest
}

func (m *mockSCMPublisher) SubmitPullRequestReview(ctx context.Context, owner, repo string, pullNumber int, submission github.PullReviewSubmission) error {
	m.submittedReviews = append(m.submittedReviews, submission)
	return nil
}

func (m *mockSCMPublisher) UpdateCheckRun(ctx context.Context, owner, repo string, checkRunID int64, req github.UpdateCheckRunRequest) error {
	m.updatedCheckRuns = append(m.updatedCheckRuns, req)
	return nil
}

func TestOrchestratorSCMReviewDispatch(t *testing.T) {
	ctx := context.Background()

	// 1. Setup minimal orchestrator with SCM publisher
	catalog := rules.DefaultCatalog()
	evaluator, _ := rules.NewEvaluator(catalog)
	aiGateway := llm.NewGateway("", "", "", "")
	artifactClient := storage.NewArtifactClient("http://localhost", "proj", "key")

	orchestrator := review.NewOrchestrator(nil, aiGateway, artifactClient, evaluator)
	mockPub := &mockSCMPublisher{}
	orchestrator.SetSCMPublisher(mockPub)

	// Sample diff triggering rule findings
	sampleDiff := `diff --git a/payments.go b/payments.go
--- a/payments.go
+++ b/payments.go
@@ -1,3 +1,4 @@
 func ProcessPayment() {
+	apiKey := "sk-live-12345678901234567890"
+	query := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", userID)
 }
`

	task := review.ExecutionTask{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "acme/payments-api",
		PullNumber:    42,
		Title:         "Add payment processing",
		HeadSHA:       "abcdef123456",
		BaseSHA:       "123456abcdef",
		Author:        "alice",
		RawDiff:       sampleDiff,
		CheckRunID:    7788,
	}

	// ProcessReview will evaluate rules, format comments, and publish to mock SCM
	_ = orchestrator.ProcessReview(ctx, task)

	if len(mockPub.submittedReviews) == 0 {
		t.Fatal("expected review to be submitted to SCM publisher")
	}

	sub := mockPub.submittedReviews[0]
	if sub.Event != "REQUEST_CHANGES" {
		t.Errorf("expected REQUEST_CHANGES event due to security finding, got %s", sub.Event)
	}
	if len(sub.Comments) == 0 {
		t.Error("expected inline comments in submitted review")
	}

	if len(mockPub.updatedCheckRuns) == 0 {
		t.Fatal("expected check run to be updated")
	}
	chk := mockPub.updatedCheckRuns[0]
	if chk.Conclusion != "failure" {
		t.Errorf("expected failure conclusion for check run, got %s", chk.Conclusion)
	}
	if chk.Output == nil || !strings.Contains(chk.Output.Summary, "actionable findings") {
		t.Errorf("unexpected check run output: %+v", chk.Output)
	}
}
