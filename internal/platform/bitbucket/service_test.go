// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package bitbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitbucketCloudService(t *testing.T) {
	mux := http.NewServeMux()

	// GET /user
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"display_name": "Test User",
			"nickname":     "testuser",
			"account_id":   "acc-123",
			"links": map[string]any{
				"avatar": map[string]string{"href": "https://avatar.url"},
			},
		})
	})

	// GET /repositories/myws/myrepo/pullrequests/42
	mux.HandleFunc("/repositories/myws/myrepo/pullrequests/42", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          42,
			"title":       "Feature PR",
			"description": "Awesome feature",
			"state":       "OPEN",
			"created_on":  "2026-03-01T12:00:00Z",
			"updated_on":  "2026-03-01T13:00:00Z",
			"author": map[string]any{
				"nickname":     "author1",
				"display_name": "Author One",
				"account_id":   "acc-auth",
			},
			"source": map[string]any{
				"branch": map[string]string{"name": "feat-1"},
				"commit": map[string]string{"hash": "head123"},
			},
			"destination": map[string]any{
				"branch": map[string]string{"name": "main"},
				"commit": map[string]string{"hash": "base123"},
			},
			"links": map[string]any{
				"html": map[string]string{"href": "https://bitbucket.org/myws/myrepo/pull-requests/42"},
			},
		})
	})

	// POST /repositories/myws/myrepo/pullrequests/42/comments
	mux.HandleFunc("/repositories/myws/myrepo/pullrequests/42/comments", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 999,
		})
	})

	// POST /repositories/myws/myrepo/pullrequests/42/approve
	mux.HandleFunc("/repositories/myws/myrepo/pullrequests/42/approve", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewBitbucketCloudService(BitbucketCloudConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		AuthToken: "test-token",
	}
	repo := &types.RepositoryDescriptor{
		Owner: "myws",
		Name:  "myrepo",
	}

	t.Run("Provider", func(t *testing.T) {
		assert.Equal(t, models.ProviderBitbucket, svc.Provider())
	})

	t.Run("VerifyConnection", func(t *testing.T) {
		status, err := svc.VerifyConnection(ctx, orgData)
		require.NoError(t, err)
		assert.True(t, status.HasConnection)
		assert.True(t, status.IsConnected)
	})

	t.Run("GetCurrentUser", func(t *testing.T) {
		u, err := svc.GetCurrentUser(ctx, orgData)
		require.NoError(t, err)
		assert.Equal(t, "testuser", u.Username)
		assert.Equal(t, "Test User", u.Name)
	})

	t.Run("GetPullRequest", func(t *testing.T) {
		pr, err := svc.GetPullRequest(ctx, orgData, repo, 42)
		require.NoError(t, err)
		assert.Equal(t, 42, pr.Number)
		assert.Equal(t, "Feature PR", pr.Title)
		assert.Equal(t, "feat-1", pr.SourceBranch)
		assert.Equal(t, "main", pr.TargetBranch)
		assert.Equal(t, "head123", pr.HeadSHA)
		assert.Equal(t, "base123", pr.BaseSHA)
	})

	t.Run("CreateReviewComment", func(t *testing.T) {
		comment, err := svc.CreateReviewComment(ctx, orgData, repo, 42, types.PullRequestReviewComment{
			Path: "main.go",
			Line: 10,
			Body: "Refactor this loop",
		})
		require.NoError(t, err)
		assert.Equal(t, "999", comment.ID)
	})

	t.Run("ApprovePullRequest", func(t *testing.T) {
		err := svc.ApprovePullRequest(ctx, orgData, repo, 42, "")
		require.NoError(t, err)
	})

	t.Run("GetCloneParams", func(t *testing.T) {
		params, err := svc.GetCloneParams(ctx, orgData, *repo)
		require.NoError(t, err)
		assert.Contains(t, params.URL, "x-token-auth:test-token@bitbucket.org/myws/myrepo.git")
	})

	t.Run("FormatReviewCommentBody", func(t *testing.T) {
		body := svc.FormatReviewCommentBody("Clean up err handling", "go", true, true)
		assert.Contains(t, body, "<!-- drixy-codereview -->")
		assert.Contains(t, body, "Clean up err handling")
		assert.Contains(t, body, "https://scandrix.dev")
		assert.NotContains(t, body, "kodus")
		assert.NotContains(t, body, "kody")
	})
}

func TestBitbucketServerService(t *testing.T) {
	mux := http.NewServeMux()

	// GET /rest/api/1.0/users?limit=1
	mux.HandleFunc("/rest/api/1.0/users", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{
				{
					"id":          1001,
					"name":        "johndoe",
					"displayName": "John Doe",
				},
			},
		})
	})

	// GET /rest/api/1.0/projects/PROJ/repos/repo1/pull-requests/10
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo1/pull-requests/10", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          10,
			"title":       "Server PR",
			"description": "Server PR description",
			"state":       "OPEN",
			"createdDate": int64(1700000000000),
			"updatedDate": int64(1700000010000),
			"fromRef": map[string]any{
				"id":           "refs/heads/feature",
				"displayId":    "feature",
				"latestCommit": "serverhead123",
			},
			"toRef": map[string]any{
				"id":           "refs/heads/master",
				"displayId":    "master",
				"latestCommit": "serverbase123",
			},
			"author": map[string]any{
				"user": map[string]string{
					"name":        "johndoe",
					"displayName": "John Doe",
				},
			},
			"links": map[string]any{
				"self": []map[string]string{{"href": "http://localhost:7990/projects/PROJ/repos/repo1/pull-requests/10"}},
			},
		})
	})

	// POST /rest/api/1.0/projects/PROJ/repos/repo1/pull-requests/10/comments
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo1/pull-requests/10/comments", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 888,
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewBitbucketServerService(BitbucketServerConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		AuthToken: "server-pat",
		IntegrationCredentials: map[string]any{
			"host": server.URL,
		},
	}
	repo := &types.RepositoryDescriptor{
		Owner: "PROJ",
		Name:  "repo1",
	}

	t.Run("VerifyConnection", func(t *testing.T) {
		status, err := svc.VerifyConnection(ctx, orgData)
		require.NoError(t, err)
		assert.True(t, status.HasConnection)
	})

	t.Run("GetPullRequest", func(t *testing.T) {
		pr, err := svc.GetPullRequest(ctx, orgData, repo, 10)
		require.NoError(t, err)
		assert.Equal(t, 10, pr.Number)
		assert.Equal(t, "Server PR", pr.Title)
		assert.Equal(t, "feature", pr.SourceBranch)
		assert.Equal(t, "master", pr.TargetBranch)
		assert.Equal(t, "serverhead123", pr.HeadSHA)
	})

	t.Run("CreateReviewComment", func(t *testing.T) {
		comment, err := svc.CreateReviewComment(ctx, orgData, repo, 10, types.PullRequestReviewComment{
			Path: "app.go",
			Line: 25,
			Body: "Check nil pointer here",
		})
		require.NoError(t, err)
		assert.Equal(t, "888", comment.ID)
	})

	t.Run("SupportsIssues", func(t *testing.T) {
		supported, err := svc.SupportsIssues(ctx, orgData)
		require.NoError(t, err)
		assert.False(t, supported, "Server does not have built-in issues")
	})

	t.Run("FormatReviewCommentBody", func(t *testing.T) {
		body := svc.FormatReviewCommentBody("Check bounds", "go", true, true)
		assert.Contains(t, body, "<!-- drixy-codereview -->")
		assert.Contains(t, body, "https://scandrix.dev")
	})
}

func TestBitbucketRouterService(t *testing.T) {
	cloud := NewBitbucketCloudService(BitbucketCloudConfig{})
	server := NewBitbucketServerService(BitbucketServerConfig{})
	router := NewBitbucketService(cloud, server)

	t.Run("Routes to Cloud by default", func(t *testing.T) {
		orgData := types.OrganizationAndTeamData{
			AuthToken: "cloud-token",
		}
		impl := router.resolveService(orgData)
		assert.Equal(t, cloud, impl)
	})

	t.Run("Routes to Server when host is custom", func(t *testing.T) {
		orgData := types.OrganizationAndTeamData{
			AuthToken: "server-token",
			IntegrationCredentials: map[string]any{
				"host": "https://git.internal-corp.com",
			},
		}
		impl := router.resolveService(orgData)
		assert.Equal(t, server, impl)
	})

	t.Run("Routes to Server when isDataCenter is true", func(t *testing.T) {
		orgData := types.OrganizationAndTeamData{
			IntegrationCredentials: map[string]any{
				"isDataCenter": true,
			},
		}
		impl := router.resolveService(orgData)
		assert.Equal(t, server, impl)
	})
}
