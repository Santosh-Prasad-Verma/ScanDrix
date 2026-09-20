package clireview_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/clireview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTraceDecisionBranchReader struct {
	record *clireview.TraceDecisionBranchRecord
	err    error
	calls  []clireview.ReadTraceDecisionBranchInput
}

func (m *mockTraceDecisionBranchReader) Read(ctx context.Context, input clireview.ReadTraceDecisionBranchInput) (*clireview.TraceDecisionBranchRecord, error) {
	m.calls = append(m.calls, input)
	if m.err != nil {
		return nil, m.err
	}
	return m.record, nil
}

func makeTraceDecision(text string, conf float64, scope []string, pinned bool) clireview.TraceContextDecision {
	return clireview.TraceContextDecision{
		CliSessionClassifiedDecision: clireview.CliSessionClassifiedDecision{
			Type:       "architectural_decision",
			Decision:   text,
			Confidence: conf,
		},
		Scope:  scope,
		Pinned: pinned,
	}
}

func bulkyDecision(id string, conf float64, pinned bool) clireview.TraceContextDecision {
	return clireview.TraceContextDecision{
		CliSessionClassifiedDecision: clireview.CliSessionClassifiedDecision{
			Type:       "architectural_decision",
			Decision:   fmt.Sprintf("%s %s", id, strings.Repeat("x", 4000)),
			Confidence: conf,
		},
		Scope:  []string{"src/billing"},
		Pinned: pinned,
	}
}

func TestBuildTraceContextPackUseCase(t *testing.T) {
	ctx := context.Background()
	orgAndTeam := clireview.OrganizationAndTeamData{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	}
	repo := clireview.RepositoryRef{ID: "repo-1", Name: "billing"}
	branch := "feat/billing"

	t.Run("returns an empty pack when nothing has been recorded", func(t *testing.T) {
		reader := &mockTraceDecisionBranchReader{}
		uc := clireview.NewBuildTraceContextPackUseCase(reader)

		result, err := uc.Execute(ctx, clireview.BuildTraceContextPackInput{
			OrganizationAndTeamData: orgAndTeam,
			Repository:              repo,
			Branch:                  branch,
			ChangedFilePaths:        []string{"src/billing/invoice.ts"},
		})
		require.NoError(t, err)
		assert.Empty(t, result.Decisions)
		assert.Equal(t, 0, result.EstimatedTokens)
		assert.Equal(t, "", clireview.RenderTraceContextPack(result.Decisions))
	})

	t.Run("returns an empty pack when the diff touches nothing", func(t *testing.T) {
		reader := &mockTraceDecisionBranchReader{
			record: &clireview.TraceDecisionBranchRecord{
				Version: 1,
				Branch:  branch,
				Decisions: []clireview.TraceContextDecision{
					makeTraceDecision("Invoice totals are cached", 0.8, []string{"src/billing"}, false),
				},
			},
		}
		uc := clireview.NewBuildTraceContextPackUseCase(reader)

		result, err := uc.Execute(ctx, clireview.BuildTraceContextPackInput{
			OrganizationAndTeamData: orgAndTeam,
			Repository:              repo,
			Branch:                  branch,
			ChangedFilePaths:        []string{},
		})
		require.NoError(t, err)
		assert.Empty(t, result.Decisions)
		assert.Empty(t, reader.calls)
	})

	t.Run("includes decisions for the changed area and none from unrelated ones", func(t *testing.T) {
		reader := &mockTraceDecisionBranchReader{
			record: &clireview.TraceDecisionBranchRecord{
				Version: 1,
				Branch:  branch,
				Decisions: []clireview.TraceContextDecision{
					makeTraceDecision("billing decision", 0.8, []string{"src/billing"}, false),
					makeTraceDecision("auth decision", 0.8, []string{"src/auth/login.ts"}, false),
					makeTraceDecision("infra decision", 0.8, []string{"infra/terraform"}, false),
				},
			},
		}
		uc := clireview.NewBuildTraceContextPackUseCase(reader)

		result, err := uc.Execute(ctx, clireview.BuildTraceContextPackInput{
			OrganizationAndTeamData: orgAndTeam,
			Repository:              repo,
			Branch:                  branch,
			ChangedFilePaths:        []string{"src/billing/invoice.ts"},
		})
		require.NoError(t, err)
		require.Len(t, result.Decisions, 1)
		assert.Equal(t, "billing decision", result.Decisions[0].Decision)

		rendered := clireview.RenderTraceContextPack(result.Decisions)
		assert.Contains(t, rendered, "billing decision")
		assert.NotContains(t, rendered, "auth decision")
		assert.NotContains(t, rendered, "infra decision")
	})

	t.Run("scopes the query to the branch when one is given", func(t *testing.T) {
		reader := &mockTraceDecisionBranchReader{}
		uc := clireview.NewBuildTraceContextPackUseCase(reader)

		_, err := uc.Execute(ctx, clireview.BuildTraceContextPackInput{
			OrganizationAndTeamData: orgAndTeam,
			Repository:              repo,
			Branch:                  "feat/billing",
			ChangedFilePaths:        []string{"src/billing/invoice.ts"},
		})
		require.NoError(t, err)
		require.Len(t, reader.calls, 1)
		assert.Equal(t, "feat/billing", reader.calls[0].Branch)
		assert.Equal(t, "repo-1", reader.calls[0].RepositoryID)
		assert.Equal(t, "org-1", reader.calls[0].OrganizationID)
	})

	t.Run("deduplicates the same decision recorded across sessions", func(t *testing.T) {
		reader := &mockTraceDecisionBranchReader{
			record: &clireview.TraceDecisionBranchRecord{
				Version: 1,
				Branch:  branch,
				Decisions: []clireview.TraceContextDecision{
					makeTraceDecision("same decision", 0.4, []string{"src/billing"}, false),
					makeTraceDecision("same decision", 0.9, []string{"src/billing"}, false),
				},
			},
		}
		uc := clireview.NewBuildTraceContextPackUseCase(reader)

		result, err := uc.Execute(ctx, clireview.BuildTraceContextPackInput{
			OrganizationAndTeamData: orgAndTeam,
			Repository:              repo,
			Branch:                  branch,
			ChangedFilePaths:        []string{"src/billing/invoice.ts"},
		})
		require.NoError(t, err)
		require.Len(t, result.Decisions, 1)
		assert.Equal(t, 0.9, result.Decisions[0].Confidence)
	})

	t.Run("returns an empty pack rather than failing when the store is unavailable", func(t *testing.T) {
		reader := &mockTraceDecisionBranchReader{
			err: errors.New("provider is down"),
		}
		uc := clireview.NewBuildTraceContextPackUseCase(reader)

		result, err := uc.Execute(ctx, clireview.BuildTraceContextPackInput{
			OrganizationAndTeamData: orgAndTeam,
			Repository:              repo,
			Branch:                  branch,
			ChangedFilePaths:        []string{"src/billing/invoice.ts"},
		})
		require.NoError(t, err)
		assert.Empty(t, result.Decisions)
	})
}

func TestMatchesAnyPath(t *testing.T) {
	d := func(scope []string) clireview.TraceContextDecision {
		return clireview.TraceContextDecision{
			Scope: scope,
		}
	}

	t.Run("matches a directory scope against a file inside it", func(t *testing.T) {
		assert.True(t, clireview.MatchesAnyPath(d([]string{"src/billing"}), []string{"src/billing/invoice.ts"}))
	})

	t.Run("matches a file scope against a changed directory", func(t *testing.T) {
		assert.True(t, clireview.MatchesAnyPath(d([]string{"src/billing/invoice.ts"}), []string{"src/billing"}))
	})

	t.Run("does not match a sibling prefix", func(t *testing.T) {
		assert.False(t, clireview.MatchesAnyPath(d([]string{"src/billing"}), []string{"src/billing-legacy/invoice.ts"}))
	})

	t.Run("normalizes leading ./ and backslashes", func(t *testing.T) {
		assert.True(t, clireview.MatchesAnyPath(d([]string{"./src\\billing"}), []string{"src/billing/invoice.ts"}))
	})

	t.Run("never matches a decision with no scope", func(t *testing.T) {
		assert.False(t, clireview.MatchesAnyPath(d([]string{}), []string{"src/billing/invoice.ts"}))
		assert.False(t, clireview.MatchesAnyPath(d(nil), []string{"src/billing/invoice.ts"}))
	})
}

func TestThe2000TokenBudget(t *testing.T) {
	t.Run("defaults to 2000 tokens", func(t *testing.T) {
		assert.Equal(t, 2000, clireview.TraceContextPackTokenBudget)
	})

	t.Run("keeps the pack within budget", func(t *testing.T) {
		result := clireview.ApplyBudget([]clireview.TraceContextDecision{
			bulkyDecision("a", 0.9, false),
			bulkyDecision("b", 0.8, false),
			bulkyDecision("c", 0.7, false),
		}, clireview.TraceContextPackTokenBudget)

		assert.LessOrEqual(t, result.EstimatedTokens, clireview.TraceContextPackTokenBudget)
		assert.Greater(t, result.DroppedForBudget, 0)
	})

	t.Run("drops the lowest confidence first", func(t *testing.T) {
		costHigh := clireview.EstimateTokens(clireview.RenderDecision(bulkyDecision("high", 0.95, false)))
		budget := costHigh + 200

		result := clireview.ApplyBudget([]clireview.TraceContextDecision{
			bulkyDecision("low", 0.1, false),
			bulkyDecision("high", 0.95, false),
			bulkyDecision("mid", 0.5, false),
		}, budget)

		require.Len(t, result.Decisions, 1)
		firstWord := strings.Split(result.Decisions[0].Decision, " ")[0]
		assert.Equal(t, "high", firstWord)
		assert.Equal(t, 2, result.DroppedForBudget)
	})

	t.Run("never drops a pinned decision, even the lowest confidence one", func(t *testing.T) {
		result := clireview.ApplyBudget([]clireview.TraceContextDecision{
			bulkyDecision("low-but-pinned", 0.01, true),
			bulkyDecision("high", 0.95, false),
		}, 10)

		var kept []string
		for _, d := range result.Decisions {
			kept = append(kept, strings.Split(d.Decision, " ")[0])
		}
		assert.Contains(t, kept, "low-but-pinned")
		assert.NotContains(t, kept, "high")
	})

	t.Run("keeps everything when it all fits", func(t *testing.T) {
		result := clireview.ApplyBudget([]clireview.TraceContextDecision{
			makeTraceDecision("a", 0.8, []string{"src/a"}, false),
			makeTraceDecision("b", 0.8, []string{"src/b"}, false),
		}, clireview.TraceContextPackTokenBudget)

		assert.Len(t, result.Decisions, 2)
		assert.Equal(t, 0, result.DroppedForBudget)
	})
}
