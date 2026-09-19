package repositories

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// FeaturedPublicReviewRepository provides persistence and querying for showcased public reviews.
type FeaturedPublicReviewRepository struct {
	mu     sync.RWMutex
	bySlug map[string]*domain.FeaturedPublicReview
}

// NewFeaturedPublicReviewRepository initializes the repository.
func NewFeaturedPublicReviewRepository() *FeaturedPublicReviewRepository {
	return &FeaturedPublicReviewRepository{
		bySlug: make(map[string]*domain.FeaturedPublicReview),
	}
}

// UpsertBySlug creates or replaces a curated review snapshot by its URL-friendly slug.
func (r *FeaturedPublicReviewRepository) UpsertBySlug(
	ctx context.Context,
	slug string,
	review *domain.FeaturedPublicReview,
) (*domain.FeaturedPublicReview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	if existing, ok := r.bySlug[slug]; ok {
		review.CreatedAt = existing.CreatedAt
	} else if review.CreatedAt.IsZero() {
		review.CreatedAt = now
	}
	review.Slug = slug
	review.UpdatedAt = now

	r.bySlug[slug] = review
	return review, nil
}

// FindBySlug returns a published featured review matching the slug, or nil if unpublished/missing.
func (r *FeaturedPublicReviewRepository) FindBySlug(ctx context.Context, slug string) (*domain.FeaturedPublicReview, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.bySlug[slug]
	if !ok || !rec.Published {
		return nil, nil
	}
	return rec, nil
}

// ListPublished provides lightweight review items for grid and home views.
func (r *FeaturedPublicReviewRepository) ListPublished(ctx context.Context) ([]domain.FeaturedPublicReviewListItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []domain.FeaturedPublicReviewListItem
	for _, doc := range r.bySlug {
		if !doc.Published {
			continue
		}

		issuesCount := 0
		if doc.Result != nil {
			if issuesRaw, ok := doc.Result["issues"].([]any); ok {
				issuesCount = len(issuesRaw)
			} else if issuesTyped, ok := doc.Result["issues"].([]domain.CliReviewIssue); ok {
				issuesCount = len(issuesTyped)
			}
		}

		items = append(items, domain.FeaturedPublicReviewListItem{
			Slug:        doc.Slug,
			Tags:        doc.Tags,
			Highlight:   doc.Highlight,
			PrURL:       doc.PrURL,
			PR:          doc.PR,
			SortOrder:   doc.SortOrder,
			IssuesCount: issuesCount,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		orderI := 999999
		if items[i].SortOrder != nil {
			orderI = *items[i].SortOrder
		}
		orderJ := 999999
		if items[j].SortOrder != nil {
			orderJ = *items[j].SortOrder
		}

		if orderI != orderJ {
			return orderI < orderJ
		}
		return items[i].Slug < items[j].Slug
	})

	return items, nil
}
