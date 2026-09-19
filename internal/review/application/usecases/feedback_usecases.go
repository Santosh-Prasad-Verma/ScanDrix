package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// CollectedReaction models an inline comment reaction observed on an SCM.
type CollectedReaction struct {
	SuggestionID string                   `json:"suggestionId"`
	RuleID       string                   `json:"ruleId,omitempty"`
	Category     domain.ReviewCategory    `json:"category,omitempty"`
	CommentID    int64                    `json:"commentId"`
	PRNumber     int                      `json:"prNumber"`
	RepoID       string                   `json:"repoId"`
	RepoFullName string                   `json:"repoFullName"`
	Reactions    domain.FeedbackReactions `json:"reactions"`
}

// IScmReactionCollector fetches developer reactions directly from GitHub/GitLab.
type IScmReactionCollector interface {
	CollectReactionsForPR(ctx context.Context, orgID string, repo domain.PRRepositoryRef, prNumber int, commentIDs []int64) ([]CollectedReaction, error)
}

// FeedbackUseCases coordinates developer feedback collection and persistence.
type FeedbackUseCases struct {
	repo      domain.ICodeReviewFeedbackRepository
	collector IScmReactionCollector
}

// NewFeedbackUseCases creates a new feedback use case coordinator.
func NewFeedbackUseCases(repo domain.ICodeReviewFeedbackRepository, collector IScmReactionCollector) *FeedbackUseCases {
	return &FeedbackUseCases{
		repo:      repo,
		collector: collector,
	}
}

// SaveFeedback syncs developer reactions from SCM into the local datastore with change detection.
func (u *FeedbackUseCases) SaveFeedback(ctx context.Context, orgID string, repo domain.PRRepositoryRef, prNumber int, commentIDs []int64) ([]CollectedReaction, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	collected, err := u.collector.CollectReactionsForPR(ctx, orgID, repo, prNumber, commentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to collect reactions: %w", err)
	}

	var changed []CollectedReaction
	now := time.Now().UTC()

	for _, item := range collected {
		existing, err := u.repo.FindByCommentID(ctx, orgID, item.CommentID)
		if err == nil && existing != nil {
			// Compare counts: only update if reactions changed (preserves analytics watermark cursor)
			if existing.Reactions.ThumbsUp == item.Reactions.ThumbsUp && existing.Reactions.ThumbsDown == item.Reactions.ThumbsDown {
				continue
			}

			existing.Reactions = item.Reactions
			existing.UpdatedAt = now
			if err := u.repo.Save(ctx, existing); err == nil {
				changed = append(changed, item)
			}
		} else {
			// New feedback record
			fb := &domain.CodeReviewFeedback{
				ID:             uuid.New(),
				OrganizationID: orgID,
				SuggestionID:   item.SuggestionID,
				RuleID:         item.RuleID,
				Category:       item.Category,
				Reactions:      item.Reactions,
				Comment: domain.CommentLocationRef{
					ID: item.CommentID,
				},
				PullRequest: domain.PRContextRef{
					Number:     item.PRNumber,
					Repository: repo,
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := u.repo.Save(ctx, fb); err == nil {
				changed = append(changed, item)
			}
		}
	}

	return changed, nil
}

// GetReactions retrieves aggregated feedback metrics and sentiment ratio for an organization or repo.
func (u *FeedbackUseCases) GetReactions(ctx context.Context, orgID string) (*domain.FeedbackAggregateMetrics, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}
	return u.repo.GetMetricsByOrg(ctx, orgID)
}
