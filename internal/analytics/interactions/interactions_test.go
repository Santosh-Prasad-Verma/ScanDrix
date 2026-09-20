package interactions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteractionService_RecordAndGet(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryInteractionRepository()
	svc := NewInteractionService(repo)

	now := time.Now().UTC()
	err := svc.RecordInteraction(ctx, InteractionExecution{
		PlatformUserID:     "user-1",
		InteractionType:    "button_click",
		InteractionCommand: "fix_issue",
		ButtonLabel:        "Apply Suggestion",
		OrganizationID:     "org-1",
		TeamID:             "team-1",
		InteractionDate:    now,
	})
	require.NoError(t, err)

	items, err := svc.GetInteractions(ctx, "org-1", now.Add(-1*time.Hour), now.Add(1*time.Hour))
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "user-1", items[0].PlatformUserID)
	assert.Equal(t, "button_click", items[0].InteractionType)
	assert.Equal(t, "Apply Suggestion", items[0].ButtonLabel)
	assert.NotEmpty(t, items[0].UUID)
}
