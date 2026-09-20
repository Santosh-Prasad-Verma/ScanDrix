package taskcontext_test

import (
	"testing"

	"github.com/scandrix/backend/internal/agents/skills/capabilities/taskcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskReferences(t *testing.T) {
	text := "Fixes PROJ-123 and addresses issue #456. See https://linear.app/team/issue/PROJ-123 and ari:cloud:jira::issue/10001"

	keys := taskcontext.ExtractIssueKeys(text)
	assert.Contains(t, keys, "PROJ-123")

	nums := taskcontext.ExtractIssueNumbers(text)
	assert.Contains(t, nums, 456)

	links := taskcontext.ExtractLinks(text)
	assert.Contains(t, links, "https://linear.app/team/issue/PROJ-123")

	aris := taskcontext.ExtractAris(text)
	assert.Contains(t, aris, "ari:cloud:jira::issue/10001")

	assert.True(t, taskcontext.IsLikelyIssueKey("ENG-99"))
	assert.False(t, taskcontext.IsLikelyIssueKey("invalid-key-"))

	assert.True(t, taskcontext.IsLikelyURL("https://scandrix.dev"))
	assert.False(t, taskcontext.IsLikelyURL("ftp://something"))

	assert.True(t, taskcontext.IsLikelyTaskReferenceUrl("https://jira.atlassian.net/browse/PROJ-1"))
	assert.False(t, taskcontext.IsLikelyTaskReferenceUrl("https://google.com/search"))
}

func TestToolAliases(t *testing.T) {
	key := taskcontext.BuildToolAliasKey("getJiraIssuesProvider")
	assert.Equal(t, "get issue jira", key)

	key2 := taskcontext.BuildToolAliasKey("search_linear_tickets_workspace")
	assert.Equal(t, "linear search ticket", key2)
}

func TestScoringAndUsability(t *testing.T) {
	valid := &taskcontext.TaskContextNormalized{
		ID:                 "PROJ-123",
		Title:              "Add authentication middleware",
		Description:        "Implement JWT token validation in API gateway.",
		AcceptanceCriteria: []string{"Must validate expiry", "Must check signature"},
		Links:              []string{"https://linear.app/issue/PROJ-123"},
	}

	score := taskcontext.ScoreNormalizedContext(valid)
	assert.Equal(t, 11, score)
	assert.True(t, taskcontext.IsUsableTaskContext(valid))

	errCandidate := &taskcontext.TaskContextNormalized{
		Title:       "Error 404",
		Description: "Status 404: Not Found",
	}
	assert.False(t, taskcontext.IsUsableTaskContext(errCandidate))

	empty := &taskcontext.TaskContextNormalized{}
	assert.False(t, taskcontext.IsUsableTaskContext(empty))
}

func TestExtractSiteHints(t *testing.T) {
	payload := map[string]any{
		"result": []any{
			map[string]any{
				"id":  "cloud-id-123",
				"url": "https://my-org.atlassian.net",
			},
		},
	}

	hints := taskcontext.ExtractSiteHints(payload)
	require.Len(t, hints.SiteIDs, 1)
	assert.Equal(t, "cloud-id-123", hints.SiteIDs[0])
	require.Len(t, hints.SiteURLs, 1)
	assert.Equal(t, "https://my-org.atlassian.net", hints.SiteURLs[0])
}

func TestBuildTaskContextArgsCandidates(t *testing.T) {
	params := taskcontext.TaskContextReadParams{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	}
	hints := taskcontext.TaskContextHints{
		IssueKeys: []string{"PROJ-123"},
		SiteIDs:   []string{"cloud-abc"},
	}
	sig := &taskcontext.TaskContextToolSignature{
		RequiredParams: []string{"cloudId", "issueIdOrKey"},
		Properties: map[string]map[string]any{
			"cloudId":        {"type": "string"},
			"issueIdOrKey":   {"type": "string"},
		},
		NormalizedProperties: map[string]map[string]any{
			"cloudid":        {"type": "string"},
			"issueidorkey":   {"type": "string"},
		},
	}

	candidates := taskcontext.BuildTaskContextArgsCandidates(params, hints, sig)
	require.NotEmpty(t, candidates)
	first := candidates[0]
	assert.Equal(t, "cloud-abc", first["cloudId"])
	assert.Equal(t, "PROJ-123", first["issueIdOrKey"])
}

func TestExtractTaskContextFromToolResult(t *testing.T) {
	payload := map[string]any{
		"fields": map[string]any{
			"key":     "PROJ-999",
			"summary": "Fix race condition in queue worker",
			"description": map[string]any{
				"type": "doc",
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{
								"type": "text",
								"text": "The worker panics when two jobs arrive concurrently.",
							},
						},
					},
				},
			},
		},
	}

	normalized := taskcontext.ExtractTaskContextFromToolResult(payload)
	require.NotNil(t, normalized)
	assert.Equal(t, "PROJ-999", normalized.ID)
	assert.Equal(t, "Fix race condition in queue worker", normalized.Title)
	assert.Contains(t, normalized.Description, "The worker panics")
}
