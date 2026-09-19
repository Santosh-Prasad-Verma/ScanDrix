// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: get_inherited_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// DrixyRuleWithInheritance decorates a rule with inheritance lineage.
type DrixyRuleWithInheritance struct {
	interfaces.DrixyRule
	Inherited string `json:"inherited,omitempty"` // "global" | "repository" | "directory"
	Excluded  bool   `json:"excluded"`
}

// InheritedDrixyRulesResponse groups applicable parent rules by hierarchical origin.
type InheritedDrixyRulesResponse struct {
	GlobalRules    []DrixyRuleWithInheritance `json:"globalRules"`
	RepoRules      []DrixyRuleWithInheritance `json:"repoRules"`
	DirectoryRules []DrixyRuleWithInheritance `json:"directoryRules"`
}

// GetInheritedDrixyRulesUseCase resolves inherited rules from global, repository, and parent directory scopes.
type GetInheritedDrixyRulesUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewGetInheritedDrixyRulesUseCase constructs the use case.
func NewGetInheritedDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
) *GetInheritedDrixyRulesUseCase {
	return &GetInheritedDrixyRulesUseCase{
		rulesService: rulesService,
	}
}

// Execute aggregates and classifies rules cascading from higher-level scopes.
func (uc *GetInheritedDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID, repositoryID, directoryID string,
) (*InheritedDrixyRulesResponse, error) {
	emptyResp := &InheritedDrixyRulesResponse{
		GlobalRules:    []DrixyRuleWithInheritance{},
		RepoRules:      []DrixyRuleWithInheritance{},
		DirectoryRules: []DrixyRuleWithInheritance{},
	}

	if repositoryID == "" || repositoryID == "global" {
		return emptyResp, nil
	}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return emptyResp, nil
	}

	globalRules := make([]DrixyRuleWithInheritance, 0)
	repoRules := make([]DrixyRuleWithInheritance, 0)
	directoryRules := make([]DrixyRuleWithInheritance, 0)

	for _, rule := range entity.Rules() {
		if rule.Status != interfaces.StatusActive {
			continue
		}

		// Normalize severity to lowercase
		rule.Severity = strings.ToLower(rule.Severity)

		isExcluded := false
		if rule.Inheritance != nil && len(rule.Inheritance.Exclude) > 0 {
			for _, exc := range rule.Inheritance.Exclude {
				if (directoryID != "" && exc == directoryID) || (repositoryID != "" && exc == repositoryID) {
					isExcluded = true
					break
				}
			}
		}

		if rule.RepositoryID == "global" || rule.RepositoryID == "" {
			globalRules = append(globalRules, DrixyRuleWithInheritance{
				DrixyRule: rule,
				Inherited: "global",
				Excluded:  isExcluded,
			})
		} else if rule.RepositoryID == repositoryID && rule.DirectoryID == "" {
			repoRules = append(repoRules, DrixyRuleWithInheritance{
				DrixyRule: rule,
				Inherited: "repository",
				Excluded:  isExcluded,
			})
		} else if rule.RepositoryID == repositoryID && rule.DirectoryID != "" && rule.DirectoryID != directoryID {
			directoryRules = append(directoryRules, DrixyRuleWithInheritance{
				DrixyRule: rule,
				Inherited: "directory",
				Excluded:  isExcluded,
			})
		}
	}

	return &InheritedDrixyRulesResponse{
		GlobalRules:    globalRules,
		RepoRules:      repoRules,
		DirectoryRules: directoryRules,
	}, nil
}
