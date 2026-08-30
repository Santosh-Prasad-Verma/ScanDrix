package rules

import (
	"github.com/scandrix/backend/internal/rules/catalog"
)

// DefaultCatalog returns the complete suite of 110+ out-of-the-box enterprise rules.
func DefaultCatalog() []RuleSpec {
	defs := catalog.GetAllCatalogRules()
	specs := make([]RuleSpec, 0, len(defs))
	for _, d := range defs {
		specs = append(specs, RuleSpec{
			ID:          d.ID,
			Name:        d.Name,
			PathPattern: d.PathPattern,
			RegexRule:   d.RegexRule,
			Severity:    d.Severity,
			Category:    d.Category,
			Description: d.Description,
			Remediation: d.Remediation,
		})
	}
	return specs
}

// CatalogByLanguage returns default rules filtered for a target language.
func CatalogByLanguage(lang string) []RuleSpec {
	defs := catalog.GetRulesByLanguage(lang)
	specs := make([]RuleSpec, 0, len(defs))
	for _, d := range defs {
		specs = append(specs, RuleSpec{
			ID:          d.ID,
			Name:        d.Name,
			PathPattern: d.PathPattern,
			RegexRule:   d.RegexRule,
			Severity:    d.Severity,
			Category:    d.Category,
			Description: d.Description,
			Remediation: d.Remediation,
		})
	}
	return specs
}

// CatalogByCategory returns default rules filtered for a specific security/defect category.
func CatalogByCategory(category string) []RuleSpec {
	defs := catalog.GetRulesByCategory(category)
	specs := make([]RuleSpec, 0, len(defs))
	for _, d := range defs {
		specs = append(specs, RuleSpec{
			ID:          d.ID,
			Name:        d.Name,
			PathPattern: d.PathPattern,
			RegexRule:   d.RegexRule,
			Severity:    d.Severity,
			Category:    d.Category,
			Description: d.Description,
			Remediation: d.Remediation,
		})
	}
	return specs
}
