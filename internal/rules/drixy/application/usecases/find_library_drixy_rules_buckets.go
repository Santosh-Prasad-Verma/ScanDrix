// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_library_drixy_rules_buckets.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
)

// FindLibraryDrixyRulesBucketsUseCase retrieves all available rule category buckets.
type FindLibraryDrixyRulesBucketsUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewFindLibraryDrixyRulesBucketsUseCase constructs the usecase.
func NewFindLibraryDrixyRulesBucketsUseCase(
	rulesService contracts.IDrixyRulesService,
) *FindLibraryDrixyRulesBucketsUseCase {
	return &FindLibraryDrixyRulesBucketsUseCase{
		rulesService: rulesService,
	}
}

// Execute queries and returns the bucket classifications.
func (uc *FindLibraryDrixyRulesBucketsUseCase) Execute(ctx context.Context) ([]contracts.BucketInfo, error) {
	buckets, err := uc.rulesService.GetLibraryDrixyRulesBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve library rule buckets: %w", err)
	}
	return buckets, nil
}
