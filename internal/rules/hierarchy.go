package rules

import (
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RuleSource defines the level at which a rule is introduced.
type RuleSource string

const (
	SourceGlobal     RuleSource = "GLOBAL"
	SourceOrg        RuleSource = "ORGANIZATION"
	SourceTeam       RuleSource = "TEAM"
	SourceRepository RuleSource = "REPOSITORY"
)

// InheritedRule represents a rule along with its inheritance provenance and active overrides.
type InheritedRule struct {
	Spec              RuleSpec   `json:"spec"`
	Source            RuleSource `json:"source"`
	SourceID          uuid.UUID  `json:"source_id"`
	IsOverridden      bool       `json:"is_overridden"`
	IsDisabled        bool       `json:"is_disabled"`
	OriginalSeverity  models.FindingSeverity `json:"original_severity"`
}

// HierarchyResolver merges rules across org, team, and repository layers according to precedence.
type HierarchyResolver struct{}

// NewHierarchyResolver initializes the rule hierarchy engine.
func NewHierarchyResolver() *HierarchyResolver {
	return &HierarchyResolver{}
}

// ResolveActiveRules computes the effective rule set for a repository.
// Precedence: Repository Rules > Team Rules > Organization Rules > Global Rules
func (h *HierarchyResolver) ResolveActiveRules(
	globalRules []RuleSpec,
	orgRules []RuleSpec,
	teamRules []RuleSpec,
	repoRules []RuleSpec,
	disabledRuleNames []string,
	severityOverrides map[string]models.FindingSeverity,
) []RuleSpec {
	disabledSet := make(map[string]bool)
	for _, d := range disabledRuleNames {
		disabledSet[strings.ToLower(strings.TrimSpace(d))] = true
	}

	ruleMap := make(map[string]RuleSpec)

	// 1. Apply Global Rules
	for _, r := range globalRules {
		ruleMap[strings.ToLower(r.Name)] = r
	}

	// 2. Override / Merge Org Rules
	for _, r := range orgRules {
		ruleMap[strings.ToLower(r.Name)] = r
	}

	// 3. Override / Merge Team Rules
	for _, r := range teamRules {
		ruleMap[strings.ToLower(r.Name)] = r
	}

	// 4. Override / Merge Repository Local Rules
	for _, r := range repoRules {
		ruleMap[strings.ToLower(r.Name)] = r
	}

	// 5. Apply Disables and Severity Overrides
	var effectiveRules []RuleSpec
	for name, r := range ruleMap {
		if disabledSet[name] {
			continue // Rule disabled for this repo
		}

		if newSev, overridden := severityOverrides[name]; overridden {
			r.Severity = newSev
		}

		effectiveRules = append(effectiveRules, r)
	}

	return effectiveRules
}
