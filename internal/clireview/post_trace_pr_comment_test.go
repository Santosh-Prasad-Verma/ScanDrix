package clireview_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/clireview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockIssueCommentManager struct {
	mu           sync.Mutex
	comments     []clireview.IssueCommentInfo
	createdCalls []string
	updatedCalls []struct {
		id   int64
		body string
	}
	listErr   error
	createErr error
	updateErr error
}

func (m *mockIssueCommentManager) ListIssueComments(ctx context.Context, orgID, teamID, repoID string, prNumber int) ([]clireview.IssueCommentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.comments, nil
}

func (m *mockIssueCommentManager) CreateIssueComment(ctx context.Context, orgID, teamID, repoID string, prNumber int, body string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return 0, m.createErr
	}
	m.createdCalls = append(m.createdCalls, body)
	id := int64(len(m.comments) + 1)
	m.comments = append(m.comments, clireview.IssueCommentInfo{ID: id, Body: body})
	return id, nil
}

func (m *mockIssueCommentManager) UpdateIssueComment(ctx context.Context, orgID, teamID, repoID string, prNumber int, commentID int64, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedCalls = append(m.updatedCalls, struct {
		id   int64
		body string
	}{id: commentID, body: body})
	for i, c := range m.comments {
		if c.ID == commentID {
			m.comments[i].Body = body
			return nil
		}
	}
	return nil
}

func makeCommentDecision(t clireview.CliSessionDecisionType, dec, rat string, scope []string) clireview.TraceContextDecision {
	return clireview.TraceContextDecision{
		CliSessionClassifiedDecision: clireview.CliSessionClassifiedDecision{
			Type:       t,
			Decision:   dec,
			Rationale:  rat,
			Confidence: 0.9,
		},
		Scope: scope,
	}
}

func TestPostTracePrCommentUseCase(t *testing.T) {
	ctx := context.Background()
	defaultDecision := makeCommentDecision(
		"tradeoff",
		"Totals are cached rather than recomputed on read",
		"Recomputing made the list view quadratic",
		[]string{"src/billing"},
	)

	baseInput := func(decisions []clireview.TraceContextDecision) clireview.PostTracePrCommentInput {
		return clireview.PostTracePrCommentInput{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			PRNumber:       42,
			RepositoryID:   "repo-1",
			RepositoryName: "scandrix-ai",
			Decisions:      decisions,
		}
	}

	t.Run("posts no comment at all when the PR has no recorded decisions", func(t *testing.T) {
		mgr := &mockIssueCommentManager{}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		outcome, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{}))
		require.NoError(t, err)
		assert.Equal(t, "skipped", outcome.Action)
		assert.Equal(t, "no-decisions", outcome.Reason)
		assert.Empty(t, mgr.createdCalls)
		assert.Empty(t, mgr.updatedCalls)
	})

	t.Run("treats a blank decision as no decision", func(t *testing.T) {
		mgr := &mockIssueCommentManager{}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		outcome, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{
			makeCommentDecision("tradeoff", "   ", "", nil),
		}))
		require.NoError(t, err)
		assert.Equal(t, "skipped", outcome.Action)
		assert.Equal(t, "no-decisions", outcome.Reason)
		assert.Empty(t, mgr.createdCalls)
	})

	t.Run("creates the comment on the first run, carrying the marker", func(t *testing.T) {
		mgr := &mockIssueCommentManager{}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		outcome, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{defaultDecision}))
		require.NoError(t, err)
		assert.Equal(t, "created", outcome.Action)
		assert.Equal(t, int64(1), outcome.CommentID)
		require.Len(t, mgr.createdCalls, 1)

		body := mgr.createdCalls[0]
		assert.Contains(t, body, clireview.TraceCommentMarker)
		assert.Contains(t, body, "Totals are cached rather than recomputed")
	})

	t.Run("updates the existing comment in place on a re-run rather than posting again", func(t *testing.T) {
		mgr := &mockIssueCommentManager{
			comments: []clireview.IssueCommentInfo{
				{ID: 99, Body: "someone else said something"},
				{ID: 123, Body: clireview.TraceCommentMarker + "\n## Why this changed"},
			},
		}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		outcome, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{defaultDecision}))
		require.NoError(t, err)
		assert.Equal(t, "updated", outcome.Action)
		assert.Equal(t, int64(123), outcome.CommentID)
		assert.Empty(t, mgr.createdCalls)
		require.Len(t, mgr.updatedCalls, 1)
		assert.Equal(t, int64(123), mgr.updatedCalls[0].id)
	})

	t.Run("leaves one comment per PR across repeated runs", func(t *testing.T) {
		mgr := &mockIssueCommentManager{}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		out1, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{defaultDecision}))
		require.NoError(t, err)
		assert.Equal(t, "created", out1.Action)

		out2, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{defaultDecision}))
		require.NoError(t, err)
		assert.Equal(t, "updated", out2.Action)

		newer := makeCommentDecision("convention", "and a newer one", "", nil)
		out3, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{newer}))
		require.NoError(t, err)
		assert.Equal(t, "updated", out3.Action)

		assert.Len(t, mgr.comments, 1)
		assert.Contains(t, mgr.comments[0].Body, "and a newer one")
	})

	t.Run("reads a GitLab-shaped comment body", func(t *testing.T) {
		mgr := &mockIssueCommentManager{
			comments: []clireview.IssueCommentInfo{
				{ID: 5, Note: clireview.TraceCommentMarker + " previous"},
			},
		}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		outcome, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{defaultDecision}))
		require.NoError(t, err)
		assert.Equal(t, "updated", outcome.Action)
		assert.Equal(t, int64(5), outcome.CommentID)
	})

	t.Run("does not post during a dry run", func(t *testing.T) {
		mgr := &mockIssueCommentManager{}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		inp := baseInput([]clireview.TraceContextDecision{defaultDecision})
		inp.DryRun = true

		outcome, err := uc.Execute(ctx, inp)
		require.NoError(t, err)
		assert.Equal(t, "skipped", outcome.Action)
		assert.Equal(t, "dry-run", outcome.Reason)
		assert.Empty(t, mgr.createdCalls)
	})

	t.Run("never lets a transport failure escape", func(t *testing.T) {
		mgr := &mockIssueCommentManager{
			listErr: errors.New("403 from the provider"),
		}
		uc := clireview.NewPostTracePrCommentUseCase(mgr)

		outcome, err := uc.Execute(ctx, baseInput([]clireview.TraceContextDecision{defaultDecision}))
		require.NoError(t, err)
		assert.Equal(t, "skipped", outcome.Action)
		assert.Equal(t, "error", outcome.Reason)
	})
}

func TestRenderTraceComment(t *testing.T) {
	d1 := makeCommentDecision(
		"tradeoff",
		"Totals are cached rather than recomputed on read",
		"Recomputing made the list view quadratic",
		[]string{"src/billing"},
	)
	d2 := makeCommentDecision(
		"convention",
		"Money is stored in cents",
		"",
		[]string{"src/billing/money.ts"},
	)

	t.Run("starts with the marker so re-runs can find it", func(t *testing.T) {
		body := clireview.RenderTraceComment([]clireview.TraceContextDecision{d1})
		assert.True(t, strings.HasPrefix(body, clireview.TraceCommentMarker))
	})

	t.Run("groups decisions by type and renders rationale and scope", func(t *testing.T) {
		body := clireview.RenderTraceComment([]clireview.TraceContextDecision{d1, d2})

		assert.Contains(t, body, "**Tradeoff**")
		assert.Contains(t, body, "**Convention**")
		assert.Contains(t, body, "Recomputing made the list view quadratic")
		assert.Contains(t, body, "`src/billing/money.ts`")
	})
}
