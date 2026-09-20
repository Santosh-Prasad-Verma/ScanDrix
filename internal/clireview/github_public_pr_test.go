package clireview_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/clireview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGitHubPublicPrService_ParseURL(t *testing.T) {
	svc := clireview.NewGitHubPublicPrService(nil)

	validCases := []struct {
		input    string
		expected *clireview.ParsedPrURL
	}{
		{
			input:    "https://github.com/sgl-project/sglang/pull/12668",
			expected: &clireview.ParsedPrURL{Owner: "sgl-project", Repo: "sglang", PRNumber: 12668},
		},
		{
			input:    "https://www.github.com/openai/codex/pull/8961",
			expected: &clireview.ParsedPrURL{Owner: "openai", Repo: "codex", PRNumber: 8961},
		},
		{
			input:    "https://github.com/microsoft/vscode/pull/240128/files",
			expected: &clireview.ParsedPrURL{Owner: "microsoft", Repo: "vscode", PRNumber: 240128},
		},
		{
			input:    "  https://github.com/scandrix-ai/scandrix-ai/pull/123  ",
			expected: &clireview.ParsedPrURL{Owner: "scandrix-ai", Repo: "scandrix-ai", PRNumber: 123},
		},
		{
			input:    "https://github.com/scandrix-ai/scandrix-ai.git/pull/42",
			expected: &clireview.ParsedPrURL{Owner: "scandrix-ai", Repo: "scandrix-ai", PRNumber: 42},
		},
	}

	for _, tc := range validCases {
		t.Run("parses "+tc.input, func(t *testing.T) {
			parsed, err := svc.ParseURL(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, parsed)
		})
	}

	invalidCases := []string{
		"not a url",
		"https://github.com/scandrix-ai/scandrix-ai",
		"https://github.com/scandrix-ai/scandrix-ai/pull/abc",
		"https://github.com/scandrix-ai/scandrix-ai/issues/1",
		"https://github.com/onlyone/pull/1",
		"https://github.com/scandrix-ai/scandrix-ai/pull/0",
	}

	for _, input := range invalidCases {
		t.Run("rejects "+input, func(t *testing.T) {
			_, err := svc.ParseURL(input)
			require.Error(t, err)
			var fetchErr *clireview.PublicPrFetchError
			require.ErrorAs(t, err, &fetchErr)
		})
	}

	otherProviders := []struct {
		input        string
		providerName string
	}{
		{"https://gitlab.com/scandrix/scandrix-ai/-/merge_requests/1", "GitLab"},
		{"https://gitlab.com/group/sub/repo/-/merge_requests/42", "GitLab"},
		{"https://bitbucket.org/scandrix/scandrix-ai/pull-requests/7", "Bitbucket"},
		{"https://dev.azure.com/scandrix/proj/_git/repo/pullrequest/9", "Azure DevOps"},
		{"https://scandrix.visualstudio.com/proj/_git/repo/pullrequest/9", "Azure DevOps"},
		{"https://github.acme-corp.com/owner/repo/pull/123", "GitHub Enterprise"},
	}

	for _, tc := range otherProviders {
		t.Run("routes "+tc.input+" to requires_auth with "+tc.providerName, func(t *testing.T) {
			_, err := svc.ParseURL(tc.input)
			require.Error(t, err)
			var fetchErr *clireview.PublicPrFetchError
			require.ErrorAs(t, err, &fetchErr)
			assert.Equal(t, "requires_auth", fetchErr.Code)
			assert.Equal(t, 403, fetchErr.StatusCode)
			assert.Contains(t, fetchErr.Message, tc.providerName)
		})
	}
}

func TestGitHubPublicPrService_Fetch(t *testing.T) {
	ctx := context.Background()

	t.Run("returns metadata and diff for a public PR", func(t *testing.T) {
		metaJSON := `{
			"title": "add feature",
			"state": "open",
			"draft": false,
			"additions": 10,
			"deletions": 2,
			"changed_files": 3,
			"commits": 1,
			"comments": 0,
			"review_comments": 0,
			"html_url": "https://github.com/o/r/pull/1",
			"head": { "sha": "aaa", "ref": "feat/x" },
			"base": {
				"sha": "bbb",
				"ref": "main",
				"repo": { "clone_url": "https://github.com/o/r.git" }
			},
			"user": { "login": "alice", "avatar_url": "https://avatars.github.com/u/1", "html_url": "https://github.com/alice" }
		}`

		client := &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				recorder := httptest.NewRecorder()
				if req.Header.Get("Accept") == "application/vnd.github.v3.diff" {
					recorder.WriteHeader(http.StatusOK)
					recorder.WriteString("diff --git a/x b/x\n+hello")
				} else {
					recorder.WriteHeader(http.StatusOK)
					recorder.WriteString(metaJSON)
				}
				return recorder.Result(), nil
			}),
		}

		svc := clireview.NewGitHubPublicPrService(client)
		res, err := svc.Fetch(ctx, "https://github.com/o/r/pull/1")
		require.NoError(t, err)
		assert.Equal(t, "add feature", res.Title)
		assert.Equal(t, "aaa", res.HeadSha)
		assert.Equal(t, "bbb", res.BaseSha)
		assert.Equal(t, "https://github.com/o/r.git", res.CloneURL)
		assert.Contains(t, res.Diff, "diff --git")
	})

	t.Run("throws requires_auth on 404", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				recorder := httptest.NewRecorder()
				recorder.WriteHeader(http.StatusNotFound)
				recorder.WriteString(`{"message": "Not Found"}`)
				return recorder.Result(), nil
			}),
		}

		svc := clireview.NewGitHubPublicPrService(client)
		_, err := svc.Fetch(ctx, "https://github.com/o/r/pull/1")
		require.Error(t, err)
		var fetchErr *clireview.PublicPrFetchError
		require.ErrorAs(t, err, &fetchErr)
		assert.Equal(t, "requires_auth", fetchErr.Code)
		assert.Equal(t, 403, fetchErr.StatusCode)
	})

	t.Run("throws too_large when additions+deletions exceed cap", func(t *testing.T) {
		metaJSON := `{
			"title": "massive PR",
			"state": "open",
			"additions": 11000,
			"deletions": 0,
			"changed_files": 5,
			"head": { "sha": "a", "ref": "x" },
			"base": { "sha": "b", "ref": "main", "repo": {} }
		}`

		client := &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				recorder := httptest.NewRecorder()
				recorder.WriteHeader(http.StatusOK)
				recorder.WriteString(metaJSON)
				return recorder.Result(), nil
			}),
		}

		svc := clireview.NewGitHubPublicPrService(client)
		_, err := svc.Fetch(ctx, "https://github.com/o/r/pull/1")
		require.Error(t, err)
		var fetchErr *clireview.PublicPrFetchError
		require.ErrorAs(t, err, &fetchErr)
		assert.Equal(t, "too_large", fetchErr.Code)
	})

	t.Run("throws rate_limited/requires_auth on 403", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				recorder := httptest.NewRecorder()
				recorder.Header().Set("x-ratelimit-remaining", "0")
				recorder.WriteHeader(http.StatusForbidden)
				recorder.WriteString(`{"message": "API rate limit exceeded"}`)
				return recorder.Result(), nil
			}),
		}

		svc := clireview.NewGitHubPublicPrService(client)
		_, err := svc.Fetch(ctx, "https://github.com/o/r/pull/1")
		require.Error(t, err)
		var fetchErr *clireview.PublicPrFetchError
		require.ErrorAs(t, err, &fetchErr)
		assert.Equal(t, "requires_auth", fetchErr.Code)
	})
}
