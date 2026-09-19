// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// InheritanceResolver resolves the effective, active custom rules for a repository review session.
type InheritanceResolver struct {
	catalog *RulesCatalog
}

// NewInheritanceResolver constructs an inheritance resolver backed by a rules catalog.
func NewInheritanceResolver(catalog *RulesCatalog) *InheritanceResolver {
	if catalog == nil {
		catalog = NewRulesCatalog()
	}
	return &InheritanceResolver{catalog: catalog}
}

// InheritanceInput encapsulates query parameters for rule resolution.
type InheritanceInput struct {
	WorkspaceID  uuid.UUID
	TeamID       *uuid.UUID
	RepositoryID string
	ChangedFiles []string
	InRepoRules  []*DrixyRule
}

// ResolveActiveRules computes active rules, applying cascading precedence and path glob filtering.
func (r *InheritanceResolver) ResolveActiveRules(ctx context.Context, input InheritanceInput) *ResolvedRuleSet {
	res := &ResolvedRuleSet{}
	if r.catalog == nil {
		return res
	}

	repoID := strings.TrimSpace(input.RepositoryID)

	// Precedence order (lower index in list = lower priority, higher index overrides)
	// 1. Library Rules
	// 2. Organization Rules
	// 3. Team Rules
	// 4. Repository Rules
	// 5. In-Repo Rules

	// We collect in stages, keying by rule key (Slug or Title) to allow overriding.
	type ruleWithTier struct {
		rule *DrixyRule
		tier int // 1=lib, 2=org, 3=team, 4=repo, 5=inrepo
	}

	mergedByKey := make(map[string]ruleWithTier)

	// Stage 1: Library
	for _, rule := range r.catalog.GetLibraryRules() {
		if rule.Status != StatusActive {
			continue
		}
		key := getRuleKey(rule)
		mergedByKey[key] = ruleWithTier{rule: rule, tier: 1}
	}

	// Stage 2: Organization
	if input.WorkspaceID != uuid.Nil {
		for _, rule := range r.catalog.GetRulesForOrg(input.WorkspaceID) {
			if rule.Status != StatusActive || !rule.Inheritable {
				continue
			}
			// Check exclusion
			if isRepoExcluded(repoID, rule.ExcludedRepos) {
				continue
			}
			if len(rule.IncludedRepos) > 0 && !isRepoIncluded(repoID, rule.IncludedRepos) {
				continue
			}

			key := getRuleKey(rule)
			if _, exists := mergedByKey[key]; exists {
				res.OverriddenCount++
			} else {
				res.InheritedCount++
			}
			mergedByKey[key] = ruleWithTier{rule: rule, tier: 2}
		}
	}

	// Stage 3: Team
	if input.WorkspaceID != uuid.Nil && input.TeamID != nil && *input.TeamID != uuid.Nil {
		for _, rule := range r.catalog.GetRulesForTeam(input.WorkspaceID, *input.TeamID) {
			if rule.Status != StatusActive {
				continue
			}
			key := getRuleKey(rule)
			if _, exists := mergedByKey[key]; exists {
				res.OverriddenCount++
			}
			mergedByKey[key] = ruleWithTier{rule: rule, tier: 3}
		}
	}

	// Stage 4: Repository
	if input.WorkspaceID != uuid.Nil && repoID != "" {
		for _, rule := range r.catalog.GetRulesForRepo(input.WorkspaceID, repoID) {
			if rule.Status != StatusActive {
				continue
			}
			key := getRuleKey(rule)
			if _, exists := mergedByKey[key]; exists {
				res.OverriddenCount++
			}
			mergedByKey[key] = ruleWithTier{rule: rule, tier: 4}
		}
	}

	// Stage 5: In-Repo Rules (.scandrix/rules)
	for _, rule := range input.InRepoRules {
		if rule.Status != StatusActive {
			continue
		}
		key := getRuleKey(rule)
		if _, exists := mergedByKey[key]; exists {
			res.OverriddenCount++
		}
		mergedByKey[key] = ruleWithTier{rule: rule, tier: 5}
	}

	// Filter by changed files path globs and partition into mechanical vs semantic
	for _, entry := range mergedByKey {
		rule := entry.rule

		if len(rule.PathGlobs) > 0 && len(input.ChangedFiles) > 0 {
			matched := false
			for _, glob := range rule.PathGlobs {
				for _, changedFile := range input.ChangedFiles {
					if matchPathGlob(glob, changedFile) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}

		res.TotalActive++
		if rule.Detector != nil {
			res.MechanicalRules = append(res.MechanicalRules, rule)
		} else {
			res.SemanticRules = append(res.SemanticRules, rule)
		}
	}

	return res
}

func getRuleKey(r *DrixyRule) string {
	if r.Slug != "" {
		return strings.ToLower(strings.TrimSpace(r.Slug))
	}
	return strings.ToLower(strings.TrimSpace(r.Title))
}

func isRepoExcluded(repoID string, excluded []string) bool {
	if repoID == "" {
		return false
	}
	for _, ex := range excluded {
		if strings.EqualFold(strings.TrimSpace(ex), repoID) {
			return true
		}
	}
	return false
}

func isRepoIncluded(repoID string, included []string) bool {
	if repoID == "" {
		return false
	}
	for _, inc := range included {
		if strings.EqualFold(strings.TrimSpace(inc), repoID) {
			return true
		}
	}
	return false
}
