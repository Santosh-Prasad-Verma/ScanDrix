package feedback

import (
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FeedbackTracker records developer sentiment and false positive flags on review comments.
type FeedbackTracker struct {
	mu        sync.RWMutex
	feedbacks map[uuid.UUID]ReviewFeedback // findingID -> feedback
}

func NewFeedbackTracker() *FeedbackTracker {
	return &FeedbackTracker{
		feedbacks: make(map[uuid.UUID]ReviewFeedback),
	}
}

// RecordReaction logs reaction changes only if the count has genuinely shifted.
// Returns true if stored feedback was updated (watermark changed), or false if unchanged.
func (t *FeedbackTracker) RecordReaction(findingID, workspaceID, repoID uuid.UUID, prNumber int, reactions map[ReactionType]int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	existing, exists := t.feedbacks[findingID]
	if exists && reflect.DeepEqual(existing.Reactions, reactions) {
		return false // No delta change, preserve timestamp watermark
	}

	thumbsUp := reactions[ReactionThumbsUp]
	thumbsDown := reactions[ReactionThumbsDown]
	confused := reactions[ReactionConfused]

	isHelpful := thumbsUp > (thumbsDown + confused)
	reportedFP := thumbsDown > 0 || confused > 0

	t.feedbacks[findingID] = ReviewFeedback{
		FindingID:    findingID,
		WorkspaceID:  workspaceID,
		RepositoryID: repoID,
		PRNumber:     prNumber,
		Reactions:    reactions,
		IsHelpful:    isHelpful,
		ReportedFP:   reportedFP,
		UpdatedAt:    time.Now().UTC(),
	}

	return true
}

// GetFeedback retrieves sentiment metadata for a specific finding.
func (t *FeedbackTracker) GetFeedback(findingID uuid.UUID) (ReviewFeedback, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	fb, exists := t.feedbacks[findingID]
	return fb, exists
}

// CalculateWorkspaceSentiment summarizes team satisfaction across all reviewed findings.
func (t *FeedbackTracker) CalculateWorkspaceSentiment(workspaceID uuid.UUID) (totalFeedback int, helpfulRate float64, fpRate float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	total := 0
	helpfulCount := 0
	fpCount := 0

	for _, fb := range t.feedbacks {
		if fb.WorkspaceID != workspaceID {
			continue
		}
		total++
		if fb.IsHelpful {
			helpfulCount++
		}
		if fb.ReportedFP {
			fpCount++
		}
	}

	if total == 0 {
		return 0, 1.0, 0.0 // Default 100% helpful when no negative votes
	}

	return total, float64(helpfulCount) / float64(total), float64(fpCount) / float64(total)
}
