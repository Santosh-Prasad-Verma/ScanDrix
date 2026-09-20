// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_validation_service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

const MaxDrixyRulesFreeQuota = 10

// RuleFilterOptions configures directory and inheritance filtering.
type RuleFilterOptions struct {
	DirectoryID  string
	RepositoryID string
	UseInclude   bool
	UseExclude   bool
}

// DrixyRulesValidationService provides path matching, glob evaluation, quota limiting, and inheritance checks.
type DrixyRulesValidationService struct {
	maxRules int
}

// NewDrixyRulesValidationService initializes the validation and filtering service.
func NewDrixyRulesValidationService() *DrixyRulesValidationService {
	return &DrixyRulesValidationService{
		maxRules: MaxDrixyRulesFreeQuota,
	}
}

// ValidateRulesLimit checks if total active rules are within allowed quota tier.
func (s *DrixyRulesValidationService) ValidateRulesLimit(limited bool, totalRules int) bool {
	if !limited {
		return true
	}
	return totalRules <= s.maxRules
}

// FilterDrixyRules filters, deduplicates, orders, and separates standard vs memory rules.
func (s *DrixyRulesValidationService) FilterDrixyRules(
	rules []interfaces.DrixyRule,
	repositoryID string,
	directoryID string,
	limited bool,
) (standardRules []interfaces.DrixyRule, memoryRules []interfaces.DrixyRule) {
	if len(rules) == 0 {
		return nil, nil
	}

	var candidateRules []interfaces.DrixyRule
	for _, rule := range rules {
		isEnforced := rule.Status == interfaces.DrixyRulesStatusActive ||
			(!limited && rule.Status == interfaces.DrixyRulesStatusPaused && rule.LockedByPlan)

		if !isEnforced {
			continue
		}

		if rule.CentralizedConfig != nil && rule.CentralizedConfig.Status == interfaces.DrixyRuleCentralizedStatusPendingAdd {
			continue
		}

		if rule.RepositoryID == "global" || rule.RepositoryID == "" {
			candidateRules = append(candidateRules, rule)
			continue
		}

		if rule.RepositoryID != repositoryID {
			continue
		}

		if directoryID != "" && rule.DirectoryID != "" {
			if rule.DirectoryID == directoryID {
				candidateRules = append(candidateRules, rule)
			}
		} else if rule.DirectoryID == "" {
			candidateRules = append(candidateRules, rule)
		}
	}

	unique := s.extractUniqueRules(candidateRules)

	sort.Slice(unique, func(i, j int) bool {
		tI := time.Time{}
		tJ := time.Time{}
		if unique[i].CreatedAt != nil {
			tI = *unique[i].CreatedAt
		}
		if unique[j].CreatedAt != nil {
			tJ = *unique[j].CreatedAt
		}
		return tI.Before(tJ)
	})

	if limited && len(unique) > s.maxRules {
		unique = unique[:s.maxRules]
	}

	for _, r := range unique {
		if r.Type == interfaces.DrixyRulesTypeMemory {
			memoryRules = append(memoryRules, r)
		} else {
			standardRules = append(standardRules, r)
		}
	}

	return standardRules, memoryRules
}

func (s *DrixyRulesValidationService) extractUniqueRules(rules []interfaces.DrixyRule) []interfaces.DrixyRule {
	seen := make(map[string]bool)
	var unique []interfaces.DrixyRule

	for _, r := range rules {
		ruleText := strings.TrimSpace(r.Rule)
		if ruleText != "" && !seen[ruleText] {
			seen[ruleText] = true
			unique = append(unique, r)
		}
	}
	return unique
}

// GetDrixyRulesForFile returns rules whose path glob pattern matches a given file.
func (s *DrixyRulesValidationService) GetDrixyRulesForFile(
	fileName string,
	rules []interfaces.DrixyRule,
	filters RuleFilterOptions,
) []interfaces.DrixyRule {
	normalizedFile := strings.TrimPrefix(strings.ReplaceAll(fileName, "\\", "/"), "/")

	return s.getApplicableRules(rules, filters, func(rule interfaces.DrixyRule) bool {
		if fileName == "" || rule.DirectoryID != "" {
			return true
		}
		rulePath := strings.TrimSpace(rule.Path)
		if rulePath == "" || rulePath == "*" || rulePath == "**/*" {
			return true
		}
		return s.isPathMatchingGlob(normalizedFile, rulePath)
	})
}

// GetDrixyRulesForFolder returns rules applicable to a specific directory path.
func (s *DrixyRulesValidationService) GetDrixyRulesForFolder(
	folderName string,
	rules []interfaces.DrixyRule,
	filters RuleFilterOptions,
) []interfaces.DrixyRule {
	normalizedFolder := strings.Trim(strings.ReplaceAll(folderName, "\\", "/"), "/")

	return s.getApplicableRules(rules, filters, func(rule interfaces.DrixyRule) bool {
		if folderName == "" || rule.DirectoryID != "" {
			return true
		}
		rulePath := strings.TrimSpace(rule.Path)
		if rulePath == "" || rulePath == "*" || rulePath == "**/*" {
			return true
		}
		globs := strings.Split(rulePath, ",")
		for _, g := range globs {
			g = strings.TrimSpace(g)
			if s.isPathMatchingGlob(normalizedFolder, g) {
				return true
			}
			base := s.getGlobBasePath(g)
			if base == "" || base == normalizedFolder || strings.HasPrefix(base, normalizedFolder+"/") {
				return true
			}
		}
		return false
	})
}

// GetMemoryRulesForContext returns active memory rules for context folder.
func (s *DrixyRulesValidationService) GetMemoryRulesForContext(
	path string,
	rules []interfaces.DrixyRule,
	filters RuleFilterOptions,
) []interfaces.DrixyRule {
	var memoryRules []interfaces.DrixyRule
	for _, r := range rules {
		if r.Type == interfaces.DrixyRulesTypeMemory && r.Status == interfaces.DrixyRulesStatusActive {
			memoryRules = append(memoryRules, r)
		}
	}
	return s.GetDrixyRulesForFolder(path, memoryRules, filters)
}

func (s *DrixyRulesValidationService) getApplicableRules(
	rules []interfaces.DrixyRule,
	filters RuleFilterOptions,
	pathMatcher func(interfaces.DrixyRule) bool,
) []interfaces.DrixyRule {
	var matched []interfaces.DrixyRule

	for _, rule := range rules {
		if filters.RepositoryID != "" && rule.RepositoryID != "global" && rule.RepositoryID != "" && rule.RepositoryID != filters.RepositoryID {
			continue
		}

		if filters.RepositoryID != "" && filters.DirectoryID == "" && rule.DirectoryID != "" {
			continue
		}

		if rule.Inheritance != nil {
			if !rule.Inheritance.Inheritable {
				continue
			}
			if filters.DirectoryID != "" && rule.DirectoryID != "" && rule.DirectoryID != filters.DirectoryID {
				isIncluded := false
				for _, inc := range rule.Inheritance.Include {
					if inc == filters.DirectoryID {
						isIncluded = true
						break
					}
				}
				if !isIncluded {
					continue
				}
			}
			if filters.UseExclude {
				isExcluded := false
				for _, exc := range rule.Inheritance.Exclude {
					if (filters.DirectoryID != "" && exc == filters.DirectoryID) ||
						(filters.RepositoryID != "" && exc == filters.RepositoryID) {
						isExcluded = true
						break
					}
				}
				if isExcluded {
					continue
				}
			}
		}

		if !pathMatcher(rule) {
			continue
		}

		matched = append(matched, rule)
	}

	return matched
}

func (s *DrixyRulesValidationService) isPathMatchingGlob(path, globPattern string) bool {
	patterns := strings.Split(globPattern, ",")
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "**/*" || p == "*" {
			return true
		}
		if match, _ := filepath.Match(p, path); match {
			return true
		}
		// Match recursive wildcard
		if strings.HasPrefix(p, "**/") {
			suffix := strings.TrimPrefix(p, "**/")
			if strings.HasSuffix(path, suffix) {
				return true
			}
			if match, _ := filepath.Match(suffix, filepath.Base(path)); match {
				return true
			}
		}
	}
	return false
}

func (s *DrixyRulesValidationService) getGlobBasePath(pattern string) string {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	var baseParts []string
	globChars := "*?[{}!"

	for _, part := range parts {
		if strings.ContainsAny(part, globChars) {
			break
		}
		baseParts = append(baseParts, part)
	}
	return strings.Join(baseParts, "/")
}
