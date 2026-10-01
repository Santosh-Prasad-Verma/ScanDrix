package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureDevOpsService(t *testing.T) {
	mux := http.NewServeMux()

	// GET /_apis/projects
	mux.HandleFunc("/_apis/projects", func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Authorization"), "Basic ")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{
					"id":   "proj-123",
					"name": "MyProject",
				},
			},
		})
	})

	// GET /proj-123/_apis/git/repositories/repo-456/pullrequests/77
	mux.HandleFunc("/proj-123/_apis/git/repositories/repo-456/pullrequests/77", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pullRequestId": 77,
			"title":         "Azure PR Title",
			"description":   "Azure PR Desc",
			"status":        "active",
			"sourceRefName": "refs/heads/feature-branch",
			"targetRefName": "refs/heads/main",
			"creationDate":  time.Now().Format(time.RFC3339),
			"createdBy": map[string]any{
				"id":          "user-1",
				"displayName": "Azure Developer",
				"uniqueName":  "dev@org.com",
			},
			"lastMergeSourceCommit": map[string]string{"objectId": "azhead123"},
			"lastMergeTargetCommit": map[string]string{"objectId": "azbase123"},
			"url": "https://dev.azure.com/myorg/MyProject/_git/MyRepo/pullrequest/77",
		})
	})

	// POST /proj-123/_apis/git/repositories/repo-456/pullRequests/77/threads
	mux.HandleFunc("/proj-123/_apis/git/repositories/repo-456/pullRequests/77/threads", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 501,
			"comments": []map[string]any{
				{
					"id":      1001,
					"content": "suggestion body",
				},
			},
		})
	})

	// PUT /proj-123/_apis/git/repositories/repo-456/pullRequests/77/reviewers/
	mux.HandleFunc("/proj-123/_apis/git/repositories/repo-456/pullRequests/77/reviewers/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewAzureDevOpsService(AzureDevOpsServiceConfig{
		HTTPClient: server.Client(),
	})

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		OrganizationID: "myorg",
		AuthToken:      "azure-pat",
		IntegrationCredentials: map[string]any{
			"org":     server.URL,
			"project": "proj-123",
		},
	}
	repo := &types.RepositoryDescriptor{
		ID:       "repo-456",
		Name:     "MyRepo",
		FullName: "proj-123/repo-456",
	}

	t.Run("Provider", func(t *testing.T) {
		assert.Equal(t, models.ProviderAzure, svc.Provider())
	})

	t.Run("VerifyConnection", func(t *testing.T) {
		status, err := svc.VerifyConnection(ctx, orgData)
		require.NoError(t, err)
		assert.True(t, status.HasConnection)
		assert.True(t, status.IsConnected)
	})

	t.Run("GetPullRequest", func(t *testing.T) {
		pr, err := svc.GetPullRequest(ctx, orgData, repo, 77)
		require.NoError(t, err)
		assert.Equal(t, 77, pr.Number)
		assert.Equal(t, "Azure PR Title", pr.Title)
		assert.Equal(t, "feature-branch", pr.SourceBranch)
		assert.Equal(t, "main", pr.TargetBranch)
		assert.Equal(t, "azhead123", pr.HeadSHA)
		assert.Equal(t, "azbase123", pr.BaseSHA)
		assert.Equal(t, "Azure Developer", pr.Author)
	})

	t.Run("CreateReviewComment", func(t *testing.T) {
		comment, err := svc.CreateReviewComment(ctx, orgData, repo, 77, types.PullRequestReviewComment{
			Path:      "src/index.ts",
			Line:      45,
			StartLine: 45,
			Body:      "Consider using const instead of let",
		})
		require.NoError(t, err)
		assert.Equal(t, "1001", comment.ID)
		assert.Equal(t, "501", comment.ThreadID)
	})

	t.Run("CreateCommentInPullRequest", func(t *testing.T) {
		comment, err := svc.CreateCommentInPullRequest(ctx, orgData, repo, 77, "General review comment")
		require.NoError(t, err)
		assert.Equal(t, "1001", comment.ID)
		assert.Equal(t, "501", comment.ThreadID)
	})

	t.Run("ApprovePullRequest", func(t *testing.T) {
		err := svc.ApprovePullRequest(ctx, orgData, repo, 77, "")
		require.NoError(t, err)
	})

	t.Run("RequestChangesPullRequest", func(t *testing.T) {
		err := svc.RequestChangesPullRequest(ctx, orgData, repo, 77, "")
		require.NoError(t, err)
	})

	t.Run("GetCloneParams", func(t *testing.T) {
		params, err := svc.GetCloneParams(ctx, orgData, *repo)
		require.NoError(t, err)
		assert.Contains(t, params.URL, "https://PAT:azure-pat@dev.azure.com/")
		assert.Equal(t, models.ProviderAzure, params.Provider)
	})

	t.Run("FormatReviewCommentBody", func(t *testing.T) {
		body := svc.FormatReviewCommentBody("Optimize SQL query", "ts", true, true)
		assert.Contains(t, body, "<!-- drixy-codereview -->")
		assert.Contains(t, body, "Optimize SQL query")
		assert.Contains(t, body, "https://scandrix.dev")
	})
}
