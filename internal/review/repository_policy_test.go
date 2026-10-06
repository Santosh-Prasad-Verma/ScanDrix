package review

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

type policyReaderFixture struct {
	settings models.RepositoryReviewSettings
	err      error
	ws, repo uuid.UUID
}

func (s *policyReaderFixture) GetRepositoryReviewSettings(_ context.Context, ws, repo uuid.UUID) (models.RepositoryReviewSettings, error) {
	s.ws, s.repo = ws, repo
	return s.settings, s.err
}

type policyPublisherSpy struct{ reviews, checks int }

func (s *policyPublisherSpy) SubmitPullRequestReview(context.Context, string, string, int, github.PullReviewSubmission) error {
	s.reviews++
	return nil
}
func (s *policyPublisherSpy) UpdateCheckRun(context.Context, string, string, int64, github.UpdateCheckRunRequest) error {
	s.checks++
	return nil
}

func TestRepositoryPolicyPreventsReviewSideEffects(t *testing.T) {
	defaults, err := models.DecodeRepositoryReviewSettings(nil, true)
	require.NoError(t, err)
	for _, tc := range []struct {
		name        string
		configure   func(*models.RepositoryReviewSettings, *ExecutionTask)
		readerError error
		wantError   bool
	}{
		{"paused", func(c *models.RepositoryReviewSettings, _ *ExecutionTask) { c.Active = false }, nil, false},
		{"automatic disabled", func(c *models.RepositoryReviewSettings, _ *ExecutionTask) { c.AutoReviewEnabled = false }, nil, false},
		{"bot excluded", func(c *models.RepositoryReviewSettings, task *ExecutionTask) {
			c.IgnoreBots = true
			task.Author = "Renovate[bot]"
		}, nil, false},
		{"target excluded", func(c *models.RepositoryReviewSettings, task *ExecutionTask) {
			c.BranchesMonitored = []string{"main"}
			task.BaseBranch = "release/main"
		}, nil, false},
		{"missing target fails closed", func(c *models.RepositoryReviewSettings, task *ExecutionTask) {
			c.BranchesMonitored = []string{"main"}
			task.BaseBranch = ""
		}, nil, true},
		{"missing author fails closed", func(c *models.RepositoryReviewSettings, task *ExecutionTask) { c.IgnoreBots = true; task.Author = "" }, nil, true},
		{"all paths ignored", func(c *models.RepositoryReviewSettings, _ *ExecutionTask) { c.IgnoredPaths = []string{"*.go"} }, nil, false},
		{"database failure", func(*models.RepositoryReviewSettings, *ExecutionTask) {}, errors.New("test database outage"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaults
			task := ExecutionTask{ReviewID: uuid.New(), WorkspaceID: uuid.New(), RepositoryID: uuid.New(), Author: "review-author", BaseBranch: "main", RepoNamespace: "test/api", PullNumber: 7, CheckRunID: 1, RawDiff: policyTestDiff("main.go", "included-marker")}
			tc.configure(&cfg, &task)
			reader := &policyReaderFixture{settings: cfg, err: tc.readerError}
			publisher := &policyPublisherSpy{}
			orchestrator := NewOrchestrator(nil, nil, nil, nil)
			orchestrator.settingsReader = reader
			orchestrator.SetSCMPublisher(publisher)
			err := orchestrator.ProcessReview(context.Background(), task)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Zero(t, publisher.reviews)
			require.Zero(t, publisher.checks)
			require.Equal(t, task.WorkspaceID, reader.ws)
			require.Equal(t, task.RepositoryID, reader.repo)
		})
	}
}

func policyTestDiff(file, marker string) string {
	return "diff --git a/" + file + " b/" + file + "\n--- a/" + file + "\n+++ b/" + file + "\n@@ -0,0 +1,1 @@\n+" + marker + "\n"
}

func TestRepositoryDiffExclusionPreservesIncludedBytes(t *testing.T) {
	first, middle, last := policyTestDiff("src/a.go", "first"), policyTestDiff("vendor/deep/b.go", "excluded"), policyTestDiff("src/c.go", "last")
	raw := first + middle + last
	require.Equal(t, raw, filterRepositoryDiff(raw, []string{"*.lock"}))
	filtered := filterRepositoryDiff(raw, []string{"vendor/**"})
	require.Equal(t, first+last, filtered)
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(filtered))
	require.NoError(t, err)
	require.Len(t, patches, 2)
	require.Equal(t, "src/c.go", patches[1].NewPath)
	require.True(t, models.MatchesRepositoryPattern("src/generated/deep/a.go", "src/*/**"))
	require.False(t, models.MatchesRepositoryPattern("vendorx/a.go", "vendor/**"))
}

func TestRepositoryDryRunFiltersLLMContextAndOverridesModel(t *testing.T) {
	var requests []struct {
		Model    string `json:"model"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	// Test-only transport: captures the real gateway request without a paid provider.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"Test transport\",\"findings\":[]}"}}]}`))
	}))
	defer server.Close()
	cfg, err := models.DecodeRepositoryReviewSettings(nil, true)
	require.NoError(t, err)
	cfg.AutoReviewEnabled, cfg.DryRunEnabled, cfg.ModelOverride = false, true, "test-configured-model"
	cfg.IgnoredPaths = []string{"vendor/**"}
	orchestrator := NewOrchestrator(nil, llm.NewGateway("", "", "", server.URL), nil, nil)
	orchestrator.settingsReader = &policyReaderFixture{settings: cfg}
	publisher := &policyPublisherSpy{}
	orchestrator.SetSCMPublisher(publisher)
	err = orchestrator.ProcessReview(context.Background(), ExecutionTask{WorkspaceID: uuid.New(), RepositoryID: uuid.New(), ReviewID: uuid.New(), Manual: true, RepoNamespace: "test/api", PullNumber: 1, HeadSHA: "test-head", CheckRunID: 2, RawDiff: policyTestDiff("src/main.go", "included-context") + policyTestDiff("vendor/secret.go", "excluded-context")})
	require.NoError(t, err)
	require.NotEmpty(t, requests, "manual review must reach the gateway even with automatic reviews off")
	for _, request := range requests {
		require.Equal(t, cfg.ModelOverride, request.Model)
		encoded, err := json.Marshal(request.Messages)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "included-context")
		require.NotContains(t, string(encoded), "excluded-context")
	}
	require.Zero(t, publisher.reviews)
	require.Zero(t, publisher.checks)
}

func TestRepositoryDiffExcludesRenamesDeletionsAndAmbiguousPaths(t *testing.T) {
	included := policyTestDiff("src/main.go", "included")
	for _, ignored := range []struct {
		name, raw, pattern string
	}{
		{"ambiguous header", policyTestDiff("private b/secret.go", "excluded"), "private b/**"},
		{"renamed source", "diff --git a/vendor/generated.go b/src/generated.go\nsimilarity index 100%\nrename from vendor/generated.go\nrename to src/generated.go\n", "vendor/**"},
		{"renamed destination", "diff --git a/src/secret.go b/vendor/secret.go\nsimilarity index 100%\nrename from src/secret.go\nrename to vendor/secret.go\n", "vendor/**"},
		{"deletion", "diff --git a/vendor/secret.go b/vendor/secret.go\ndeleted file mode 100644\n--- a/vendor/secret.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-excluded\n", "vendor/**"},
	} {
		t.Run(ignored.name, func(t *testing.T) {
			require.Equal(t, included, filterRepositoryDiff(ignored.raw+included, []string{ignored.pattern}))
		})
	}
}
