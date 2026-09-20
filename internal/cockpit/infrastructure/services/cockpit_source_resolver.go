package services

import (
	"context"

	"github.com/scandrix/backend/internal/cockpit/domain"
)

// CockpitSourceResolver determines which analytical backend serves cockpit data.
type CockpitSourceResolver struct{}

// NewCockpitSourceResolver creates an initialized source resolver.
func NewCockpitSourceResolver() *CockpitSourceResolver {
	return &CockpitSourceResolver{}
}

// Resolve returns the primary analytical source for an organization.
func (r *CockpitSourceResolver) Resolve(ctx context.Context, organizationID string) (domain.CockpitSource, error) {
	return domain.CockpitSourceInternal, nil
}
