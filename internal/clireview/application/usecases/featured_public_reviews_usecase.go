package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// GetFeaturedPublicReviewInput parameters to fetch single showcase item.
type GetFeaturedPublicReviewInput struct {
	Slug string `json:"slug"`
}

// GetFeaturedPublicReviewResult formatted output for public showcase.
type GetFeaturedPublicReviewResult struct {
	Slug      string         `json:"slug"`
	Tags      []string       `json:"tags"`
	Highlight string         `json:"highlight,omitempty"`
	PrURL     string         `json:"prUrl"`
	PR        map[string]any `json:"pr"`
	Diff      string         `json:"diff"`
	Result    any            `json:"result"`
}

// GetFeaturedPublicReviewUseCase reads single curated review by URL slug.
type GetFeaturedPublicReviewUseCase struct {
	repo domain.IFeaturedPublicReviewRepository
}

// NewGetFeaturedPublicReviewUseCase creates an initialized usecase.
func NewGetFeaturedPublicReviewUseCase(repo domain.IFeaturedPublicReviewRepository) *GetFeaturedPublicReviewUseCase {
	return &GetFeaturedPublicReviewUseCase{repo: repo}
}

// Execute retrieves review by slug.
func (uc *GetFeaturedPublicReviewUseCase) Execute(ctx context.Context, input GetFeaturedPublicReviewInput) (*GetFeaturedPublicReviewResult, error) {
	if uc.repo == nil {
		return nil, fmt.Errorf("featured reviews repository unavailable")
	}

	doc, err := uc.repo.FindBySlug(ctx, input.Slug)
	if err != nil || doc == nil {
		return nil, nil
	}

	return &GetFeaturedPublicReviewResult{
		Slug:      doc.Slug,
		Tags:      doc.Tags,
		Highlight: doc.Highlight,
		PrURL:     doc.PrURL,
		PR:        doc.PR,
		Diff:      doc.Diff,
		Result:    doc.Result,
	}, nil
}

// ListFeaturedPublicReviewsUseCase lists all published showcase cards.
type ListFeaturedPublicReviewsUseCase struct {
	repo domain.IFeaturedPublicReviewRepository
}

// NewListFeaturedPublicReviewsUseCase creates an initialized listing usecase.
func NewListFeaturedPublicReviewsUseCase(repo domain.IFeaturedPublicReviewRepository) *ListFeaturedPublicReviewsUseCase {
	return &ListFeaturedPublicReviewsUseCase{repo: repo}
}

// Execute returns lightweight showcase cards.
func (uc *ListFeaturedPublicReviewsUseCase) Execute(ctx context.Context) ([]domain.FeaturedPublicReviewListItem, error) {
	if uc.repo == nil {
		return []domain.FeaturedPublicReviewListItem{}, nil
	}
	return uc.repo.ListPublished(ctx)
}
