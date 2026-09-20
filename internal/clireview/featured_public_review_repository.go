package clireview

import (
	"github.com/scandrix/backend/internal/clireview/infrastructure/repositories"
)

// FeaturedPublicReviewRepository provides persistence and querying for showcased public reviews.
type FeaturedPublicReviewRepository = repositories.FeaturedPublicReviewRepository

// NewFeaturedPublicReviewRepository initializes the repository.
func NewFeaturedPublicReviewRepository() *FeaturedPublicReviewRepository {
	return repositories.NewFeaturedPublicReviewRepository()
}
