package skills_test

import (
	"testing"
	"time"

	"github.com/scandrix/backend/internal/agents/skills"
	"github.com/stretchr/testify/assert"
)

func TestCapabilityStrategyService_PromotionAndScoring(t *testing.T) {
	strategy := skills.NewCapabilityStrategyService()

	scope := skills.CapabilityStrategyScope{
		OrganizationID: "org-123",
		TeamID:         "team-456",
		SkillName:      "business-rules-validation",
		Capability:     "task.context.read",
	}

	candidates := []string{"jira_get_issue", "linear_get_issue", "github_get_issue"}

	// 1. Initial state: no preferred tool
	pref, found := strategy.GetPreferredTool(scope, candidates)
	assert.False(t, found)
	assert.Empty(t, pref)

	// 2. Record 1 success for jira_get_issue
	strategy.RecordExecution(skills.CapabilityExecutionTrace{
		Scope:      scope,
		ToolName:   "jira_get_issue",
		Status:     "success",
		LatencyMs:  120,
		OccurredAt: time.Now().UTC(),
	})

	// Not yet promoted (needs >= 3 successes)
	pref, found = strategy.GetPreferredTool(scope, candidates)
	assert.True(t, found, "Scored highest based on 1 success")
	assert.Equal(t, "jira_get_issue", pref)

	// 3. Record 2 more successes for jira_get_issue -> now 3 successes, 0 failures (100% rate)
	for i := 0; i < 2; i++ {
		strategy.RecordExecution(skills.CapabilityExecutionTrace{
			Scope:      scope,
			ToolName:   "jira_get_issue",
			Status:     "success",
			LatencyMs:  100,
			OccurredAt: time.Now().UTC(),
		})
	}

	// Should now be promoted
	pref, found = strategy.GetPreferredTool(scope, candidates)
	assert.True(t, found)
	assert.Equal(t, "jira_get_issue", pref)

	// 4. Test caching tools
	cached := strategy.GetCachedTools(scope)
	assert.Nil(t, cached)

	strategy.SaveCachedTools(scope, []string{"jira_get_issue", "linear_get_issue"})
	cached = strategy.GetCachedTools(scope)
	assert.Equal(t, []string{"jira_get_issue", "linear_get_issue"}, cached)
}
