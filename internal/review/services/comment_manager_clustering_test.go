package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestionClusterEngine_ClusterFindings(t *testing.T) {
	engine := NewSuggestionClusterEngine()
	ctx := context.Background()

	t.Run("Clusters Recurring Vulnerabilities Across Multiple Files", func(t *testing.T) {
		findings := []models.CodeFinding{
			{
				ID:            uuid.New(),
				FilePath:      "pkg/auth/jwt.go",
				StartLine:     45,
				EndLine:       46,
				Severity:      models.SeverityCritical,
				Category:      "security",
				Title:         "Hardcoded JWT Secret",
				SuggestedDiff: `secret := os.Getenv("JWT_SECRET")`,
			},
			{
				ID:            uuid.New(),
				FilePath:      "pkg/auth/oauth.go",
				StartLine:     112,
				EndLine:       113,
				Severity:      models.SeverityHigh,
				Category:      "security",
				Title:         "Hardcoded JWT Secret",
				SuggestedDiff: `secret := os.Getenv("JWT_SECRET")`,
			},
			{
				ID:            uuid.New(),
				FilePath:      "pkg/auth/middleware.go",
				StartLine:     28,
				EndLine:       29,
				Severity:      models.SeverityMedium,
				Category:      "security",
				Title:         "Hardcoded JWT Secret",
				SuggestedDiff: `secret := os.Getenv("JWT_SECRET")`,
			},
			{
				ID:            uuid.New(),
				FilePath:      "pkg/db/pool.go",
				StartLine:     10,
				EndLine:       10,
				Severity:      models.SeverityLow,
				Category:      "performance",
				Title:         "Idle Connection Limit",
				SuggestedDiff: `db.SetMaxIdleConns(10)`,
			},
		}

		masters, standalone := engine.ClusterFindings(ctx, findings)
		require.Len(t, masters, 1)
		require.Len(t, standalone, 1)

		master := masters[0]
		assert.Equal(t, models.SeverityCritical, master.Severity) // Highest severity picked
		assert.Equal(t, "Hardcoded JWT Secret", master.MasterFinding.Title)
		assert.Len(t, master.RelatedFindings, 2)
		assert.Len(t, master.Locations, 3)

		// Check formatted body includes table
		assert.Contains(t, master.FormattedMasterBody, "### 📍 Affected Locations (3 occurrences)")
		assert.Contains(t, master.FormattedMasterBody, "`pkg/auth/jwt.go`")
		assert.Contains(t, master.FormattedMasterBody, "`pkg/auth/oauth.go`")
		assert.Contains(t, master.FormattedMasterBody, "`pkg/auth/middleware.go`")
		assert.Contains(t, master.FormattedMasterBody, "```suggestion")

		// Check wire schema round-trip
		payload := engine.ConvertToRepeatedClusteringPayload(masters)
		require.Len(t, payload.CodeSuggestions, 1)
		assert.Equal(t, master.MasterID, payload.CodeSuggestions[0].ID)
		assert.Len(t, payload.CodeSuggestions[0].SameSuggestionsID, 2)
	})

	t.Run("Empty Findings Returns Empty", func(t *testing.T) {
		masters, standalone := engine.ClusterFindings(ctx, nil)
		assert.Empty(t, masters)
		assert.Empty(t, standalone)
	})
}

func TestParseRepeatedClusteringPayload(t *testing.T) {
	jsonWire := `{
		"codeSuggestions": [
			{
				"id": "c1f8a848-1234-4567-89ab-cdef01234567",
				"sameSuggestionsId": ["a2b3c4d5-1111-2222-3333-444455556666"],
				"problemDescription": "Missing nil check before pointer dereference",
				"actionStatement": "Add explicit nil validation check before accessing fields"
			}
		]
	}`

	payload, err := ParseRepeatedClusteringPayload(jsonWire)
	require.NoError(t, err)
	require.Len(t, payload.CodeSuggestions, 1)
	assert.Equal(t, "c1f8a848-1234-4567-89ab-cdef01234567", payload.CodeSuggestions[0].ID)
	assert.Equal(t, []string{"a2b3c4d5-1111-2222-3333-444455556666"}, payload.CodeSuggestions[0].SameSuggestionsID)
	assert.Equal(t, "Missing nil check before pointer dereference", payload.CodeSuggestions[0].ProblemDescription)
}

type mockThreadPageFetcher struct {
	pages map[int][]CommentThreadState
}

func (m *mockThreadPageFetcher) FetchCommentThreadPage(
	ctx context.Context, repo string, pull int, cursor PaginationCursor,
) (*PaginatedThreadPage, error) {
	threads := m.pages[cursor.CurrentPage]
	hasNext := cursor.CurrentPage < len(m.pages)
	nextToken := ""
	if hasNext {
		nextToken = fmt.Sprintf("cursor-%d", cursor.CurrentPage+1)
	}

	return &PaginatedThreadPage{
		Threads: threads,
		Cursor: PaginationCursor{
			PageSize:      len(threads),
			CurrentPage:   cursor.CurrentPage,
			NextPageToken: nextToken,
			HasNextPage:   hasNext,
			TotalPages:    len(m.pages),
		},
	}, nil
}

func TestPaginatedThreadSync_AllPages(t *testing.T) {
	mockFetcher := &mockThreadPageFetcher{
		pages: map[int][]CommentThreadState{
			1: {
				{ThreadID: "th-1", FilePath: "a.go", Line: 10},
				{ThreadID: "th-2", FilePath: "b.go", Line: 20},
			},
			2: {
				{ThreadID: "th-3", FilePath: "c.go", Line: 30},
				{ThreadID: "th-1", FilePath: "a.go", Line: 10}, // Duplicate across page boundary
			},
			3: {
				{ThreadID: "th-4", FilePath: "d.go", Line: 40},
			},
		},
	}

	sync := NewPaginatedThreadSync(mockFetcher, 10)
	threads, err := sync.SyncAllCommentThreads(context.Background(), "owner/repo", 42)
	require.NoError(t, err)
	assert.Len(t, threads, 4) // Deduplicated th-1
}

func TestSCMCommentTruncationFitter_FitAndRepair(t *testing.T) {
	fitter := NewSCMCommentTruncationFitter(models.ProviderGitHub)

	t.Run("Short Comment Stays Untouched", func(t *testing.T) {
		body := "### ScanDrix Summary\n\nAll checks passed cleanly."
		res := fitter.FitComment(body)
		assert.Equal(t, body, res)
	})

	t.Run("Oversized Comment Truncated with Notice and Code Block Repaired", func(t *testing.T) {
		// Create a string larger than GitHub 65536 limit
		var sb strings.Builder
		sb.WriteString("```go\nfunc heavy() {\n")
		for i := 0; i < 4000; i++ {
			sb.WriteString(fmt.Sprintf("\tfmt.Println(\"large payload row %04d\")\n", i))
		}
		sb.WriteString("}\n```\n")

		longBody := sb.String()
		assert.Greater(t, len(longBody), 65536)

		res := fitter.FitComment(longBody)
		assert.LessOrEqual(t, len(res), 65536)
		assert.Contains(t, res, "⚠️ Content truncated due to github comment character limit")

		// Verify Markdown code fences are balanced (even count)
		fences := strings.Count(res, "```")
		assert.True(t, fences%2 == 0, "code fences must be closed and balanced")
	})

	t.Run("RepairUnclosedMarkdown Details and Tables", func(t *testing.T) {
		broken := "<details open>\n<summary>Vulnerabilities</summary>\n<table>\n<tr><td>Row</td></tr>"
		repaired := RepairUnclosedMarkdown(broken)
		assert.Contains(t, repaired, "</table>")
		assert.Contains(t, repaired, "</details>")
	})
}
