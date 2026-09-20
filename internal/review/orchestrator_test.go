package review_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/sandbox/lease"
	"github.com/scandrix/backend/internal/sandbox/null"
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
+	api_key := "dummy_insecure_hardcoded_token_12345"
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

func TestOrchestratorAISynthesisFailure_ZeroFindings(t *testing.T) {
	ctx := context.Background()
	catalog := rules.DefaultCatalog()
	evaluator, _ := rules.NewEvaluator(catalog)

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": {"message": "The model ` + "`" + `claude-nonexistent` + "`" + ` does not exist", "code": 404}}`))
	}))
	defer failingServer.Close()

	aiGateway := llm.NewGateway("", "", "", failingServer.URL)
	artifactClient := storage.NewArtifactClient("http://localhost", "proj", "key")

	orchestrator := review.NewOrchestrator(nil, aiGateway, artifactClient, evaluator)
	mockPub := &mockSCMPublisher{}
	orchestrator.SetSCMPublisher(mockPub)

	cleanDiff := `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+func Hello() string { return "world" }
`
	task := review.ExecutionTask{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepoNamespace: "acme/clean-service",
		PullNumber:    99,
		HeadSHA:       "abcdef123456",
		RawDiff:       cleanDiff,
		CheckRunID:    9988,
	}

	_ = orchestrator.ProcessReview(ctx, task)

	if len(mockPub.updatedCheckRuns) == 0 {
		t.Fatal("expected check run to be updated")
	}
	chk := mockPub.updatedCheckRuns[0]

	// Fail-closed verification: must NOT be success!
	if chk.Conclusion != "failure" {
		t.Errorf("expected failure conclusion when AI synthesis fails, got %s", chk.Conclusion)
	}
	if chk.Output == nil || !strings.Contains(chk.Output.Summary, "ScanDrix AI Review Incomplete") {
		t.Errorf("expected summary to indicate incomplete AI review, got: %+v", chk.Output)
	}
	if !strings.Contains(chk.Output.Summary, "Provider said:") {
		t.Errorf("expected summary to contain provider diagnostics, got: %s", chk.Output.Summary)
	}
	if !strings.Contains(chk.Output.Summary, "HTTP 404") {
		t.Errorf("expected summary to contain HTTP status code, got: %s", chk.Output.Summary)
	}
}

func TestOrchestratorSandboxPipelineIntegration(t *testing.T) {
	ctx := context.Background()
	catalog := rules.DefaultCatalog()
	evaluator, _ := rules.NewEvaluator(catalog)
	aiGateway := llm.NewGateway("", "", "", "")
	artifactClient := storage.NewArtifactClient("http://localhost", "proj", "key")

	orchestrator := review.NewOrchestrator(nil, aiGateway, artifactClient, evaluator)

	repo := lease.NewMemorySandboxLeaseRepository()
	provider := null.NewNullSandboxProvider()
	leaseMgr := lease.NewSandboxLeaseManager(provider, repo, nil)

	orchestrator.SetSandboxLeaseManager(leaseMgr)
	if orchestrator.SandboxLeaseManager() != leaseMgr {
		t.Fatalf("expected sandbox lease manager to match")
	}

	pipelineEngine := orchestrator.BuildPipelineEngine(evaluator)
	if pipelineEngine == nil {
		t.Fatal("expected pipeline engine to be constructed")
	}

	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "acme/service",
		PullNumber:    12,
		RawDiff:       "diff --git a/a.go b/a.go\nnew file mode 100644\n--- /dev/null\n+++ b/a.go\n@@ -0,0 +1,1 @@\n+package main\n",
	}

	err := orchestrator.ProcessReviewWithPipeline(ctx, pCtx)
	if err != nil {
		t.Fatalf("ProcessReviewWithPipeline failed: %v", err)
	}

	if pCtx.SandboxLeaseID == "" {
		t.Error("expected sandbox lease ID to be populated in pipeline context")
	}
}

