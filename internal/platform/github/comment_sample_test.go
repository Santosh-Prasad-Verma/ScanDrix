package github

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestNumberFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected int
		valid    bool
	}{
		{"https://api.github.com/repos/scandrix/backend/pulls/42", 42, true},
		{"https://api.github.com/repos/scandrix/backend/issues/108", 108, true},
		{"https://api.github.com/repos/scandrix/backend/pulls/999?page=1", 999, true},
		{"https://api.github.com/repos/scandrix/backend/pulls/777#section", 777, true},
		{"https://api.github.com/repos/scandrix/backend/commits/abcdef", 0, false},
		{"", 0, false},
		{"not-a-url", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			num, ok := PullRequestNumberFromURL(tt.url)
			assert.Equal(t, tt.valid, ok)
			assert.Equal(t, tt.expected, num)
		})
	}
}

func TestIsPullRequestIssueComment(t *testing.T) {
	assert.True(t, IsPullRequestIssueComment("https://github.com/scandrix/backend/pull/42#issuecomment-12345"))
	assert.True(t, IsPullRequestIssueComment("https://github.com/scandrix/backend/pull/1"))
	assert.False(t, IsPullRequestIssueComment("https://github.com/scandrix/backend/issues/42#issuecomment-12345"))
	assert.False(t, IsPullRequestIssueComment("https://github.com/scandrix/backend/issues/99"))
	assert.False(t, IsPullRequestIssueComment(""))
}

func TestGroupCommentsByPullRequest(t *testing.T) {
	now := time.Now()

	reviewComments := []RawReviewComment{
		{
			ID:             1,
			PullRequestURL: "https://api.github.com/repos/scandrix/backend/pulls/10",
			Path:           "pkg/auth/jwt.go",
			Body:           "Consider constant time comparison",
			User:           "alice",
			CreatedAt:      now,
		},
		{
			ID:             2,
			PullRequestURL: "https://api.github.com/repos/scandrix/backend/pulls/10",
			Path:           "pkg/auth/jwt_test.go",
			Body:           "Add test for expired tokens",
			User:           "bob",
			CreatedAt:      now,
		},
		{
			ID:             3,
			PullRequestURL: "https://api.github.com/repos/scandrix/backend/pulls/20",
			Path:           "cmd/server/main.go",
			Body:           "Nice cleanup",
			User:           "alice",
			CreatedAt:      now,
		},
	}

	issueComments := []RawIssueComment{
		{
			ID:        101,
			IssueURL:  "https://api.github.com/repos/scandrix/backend/issues/10",
			HTMLURL:   "https://github.com/scandrix/backend/pull/10#issuecomment-101",
			Body:      "LGTM, running CI",
			User:      "charlie",
			CreatedAt: now,
		},
		{
			ID:        102,
			IssueURL:  "https://api.github.com/repos/scandrix/backend/issues/999",
			HTMLURL:   "https://github.com/scandrix/backend/issues/999#issuecomment-102", // Standalone issue comment (not PR)
			Body:      "This is a bug report discussion",
			User:      "dave",
			CreatedAt: now,
		},
	}

	grouped := GroupCommentsByPullRequest(reviewComments, issueComments)
	require.Len(t, grouped, 2)

	// PR 10
	pr10 := grouped[0]
	assert.Equal(t, 10, pr10.PR.PullNumber)
	assert.Len(t, pr10.ReviewComments, 2)
	assert.Len(t, pr10.GeneralComments, 1)
	assert.Equal(t, "LGTM, running CI", pr10.GeneralComments[0].Body)
	assert.Len(t, pr10.Files, 2)
	assert.Equal(t, "pkg/auth/jwt.go", pr10.Files[0].Filename)
	assert.Equal(t, "pkg/auth/jwt_test.go", pr10.Files[1].Filename)

	// PR 20
	pr20 := grouped[1]
	assert.Equal(t, 20, pr20.PR.PullNumber)
	assert.Len(t, pr20.ReviewComments, 1)
	assert.Empty(t, pr20.GeneralComments)
	assert.Len(t, pr20.Files, 1)
	assert.Equal(t, "cmd/server/main.go", pr20.Files[0].Filename)
}
