// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_repository.go
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// RepositoryRuleCount details rule tallies by repository and optional directory.
type RepositoryRuleCount struct {
	RepositoryID string  `json:"repositoryId"`
	DirectoryID  *string `json:"directoryId"`
	Count        int     `json:"count"`
}

// IDrixyRulesRepository manages persistence of organization rules collections.
type IDrixyRulesRepository interface {
	Create(ctx context.Context, drixyRules *interfaces.DrixyRules) (*entities.DrixyRulesEntity, error)
	FindByID(ctx context.Context, uuid string) (*interfaces.DrixyRule, error)
	FindByOrganizationID(ctx context.Context, organizationID string) (*entities.DrixyRulesEntity, error)
	Find(ctx context.Context, organizationID string, filter map[string]any) ([]*entities.DrixyRulesEntity, error)
	FindOrganizationIDsWithRules(ctx context.Context) ([]string, error)
	CountRules(ctx context.Context, organizationID string, status *interfaces.DrixyRulesStatus) (int, error)
	CountRulesByRepository(ctx context.Context, organizationID string, statuses []interfaces.DrixyRulesStatus) ([]RepositoryRuleCount, error)
	Save(ctx context.Context, entity *entities.DrixyRulesEntity) error
	DeleteRule(ctx context.Context, organizationID string, ruleUUID string) error
}
