// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_entity.go
// ═══════════════════════════════════════════════════════════════

package entities

import (
	"time"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// DrixyRulesEntity represents the domain aggregate root managing an organization's rule collection.
type DrixyRulesEntity struct {
	uuid           string
	organizationID string
	rules          []interfaces.DrixyRule
	createdAt      *time.Time
	updatedAt      *time.Time
}

// NewDrixyRulesEntity constructs a domain entity for an organization's rules.
func NewDrixyRulesEntity(data interfaces.DrixyRules) *DrixyRulesEntity {
	e := &DrixyRulesEntity{
		uuid:           data.UUID,
		organizationID: data.OrganizationID,
		rules:          data.Rules,
		createdAt:      data.CreatedAt,
		updatedAt:      data.UpdatedAt,
	}
	e.rules = e.normalizeRules(e.rules)
	return e
}

func (e *DrixyRulesEntity) normalizeRules(rules []interfaces.DrixyRule) []interfaces.DrixyRule {
	normalized := make([]interfaces.DrixyRule, len(rules))
	for i, r := range rules {
		if r.Scope == "" {
			r.Scope = interfaces.DrixyRulesScopeFile
		}
		if r.Type == "" {
			r.Type = interfaces.DrixyRulesTypeStandard
		}
		normalized[i] = r
	}
	return normalized
}

// UUID returns the unique aggregate identifier.
func (e *DrixyRulesEntity) UUID() string {
	return e.uuid
}

// OrganizationID returns the tenant identifier.
func (e *DrixyRulesEntity) OrganizationID() string {
	return e.organizationID
}

// Rules returns a defensive copy of normalized rules.
func (e *DrixyRulesEntity) Rules() []interfaces.DrixyRule {
	if e.rules == nil {
		return []interfaces.DrixyRule{}
	}
	copied := make([]interfaces.DrixyRule, len(e.rules))
	copy(copied, e.rules)
	return copied
}

// CreatedAt returns creation timestamp.
func (e *DrixyRulesEntity) CreatedAt() *time.Time {
	return e.createdAt
}

// UpdatedAt returns last update timestamp.
func (e *DrixyRulesEntity) UpdatedAt() *time.Time {
	return e.updatedAt
}

// CountActiveRules counts all active non-deleted rules.
func (e *DrixyRulesEntity) CountActiveRules() int {
	count := 0
	for _, r := range e.rules {
		if r.Status == interfaces.DrixyRulesStatusActive {
			count++
		}
	}
	return count
}

// AddOrUpdateRule inserts or replaces a rule in the entity's collection.
func (e *DrixyRulesEntity) AddOrUpdateRule(rule interfaces.DrixyRule) {
	if rule.Scope == "" {
		rule.Scope = interfaces.DrixyRulesScopeFile
	}
	if rule.Type == "" {
		rule.Type = interfaces.DrixyRulesTypeStandard
	}

	found := false
	for i, r := range e.rules {
		if r.UUID == rule.UUID {
			e.rules[i] = rule
			found = true
			break
		}
	}
	if !found {
		e.rules = append(e.rules, rule)
	}
	now := time.Now().UTC()
	e.updatedAt = &now
}

// DeleteRule removes a rule or marks it deleted.
func (e *DrixyRulesEntity) DeleteRule(uuid string) bool {
	for i, r := range e.rules {
		if r.UUID == uuid {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			now := time.Now().UTC()
			e.updatedAt = &now
			return true
		}
	}
	return false
}

// ToObject serializes entity back to domain interface representation.
func (e *DrixyRulesEntity) ToObject() interfaces.DrixyRules {
	return interfaces.DrixyRules{
		UUID:           e.uuid,
		OrganizationID: e.organizationID,
		Rules:          e.Rules(),
		CreatedAt:      e.createdAt,
		UpdatedAt:      e.updatedAt,
	}
}
