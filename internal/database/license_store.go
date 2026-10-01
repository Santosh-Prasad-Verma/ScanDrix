// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - License Store Adapter
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
)

// LicenseStore adapts the billing repository to the minimal shape the license
// package needs, so that package stays free of a dependency on the ORM models.
//
// It lives here rather than in a composition root because both the API and the
// worker must resolve entitlements through the same adapter: two adapters would
// be free to disagree about what a workspace is entitled to, which is the exact
// class of bug the shared Resolver exists to prevent.
type LicenseStore struct {
	repo *Repository
}

// NewLicenseStore wraps a repository as a license.Store. A nil repository
// yields a store that reports "no license", which resolves to Community.
func NewLicenseStore(repo *Repository) *LicenseStore {
	return &LicenseStore{repo: repo}
}

// GetActiveLicense returns the stored license row for a workspace.
func (s *LicenseStore) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*license.StoredLicense, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}

	lic, err := s.repo.GetActiveLicense(ctx, wsID)
	if err != nil || lic == nil {
		return nil, err
	}

	return &license.StoredLicense{
		Tier:     lic.PlanTier,
		Features: lic.FeaturesEnabled,
		MaxSeats: lic.TotalSeats,
		// MaxRepos is left at 0 (unlimited) deliberately, matching the adapter
		// the API has always used: the organization_licenses table carries no
		// repository cap column. Per-tier caps live in plan_configurations and are
		// applied through the plan quota, not through the license row. Deriving a
		// cap here instead would make the gate refuse reviews on tiers whose
		// repository count cannot be measured yet.
		MaxRepos:   0,
		ExpiresAt:  lic.ExpiresAt,
		CustomerNm: lic.OrganizationName,
	}, nil
}

// Compile-time proof the adapter is usable as a license.Store.
var _ license.Store = (*LicenseStore)(nil)
