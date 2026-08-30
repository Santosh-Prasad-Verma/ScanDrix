package catalog

import (
	"strings"
)

// GetAllCatalogRules returns all 110+ comprehensive out-of-the-box rule definitions.
func GetAllCatalogRules() []RuleDefinition {
	var all []RuleDefinition
	all = append(all, GetOWASPRules()...)
	all = append(all, GetDockerRules()...)
	all = append(all, GetCloudRules()...)
	all = append(all, GetGolangRules()...)
	all = append(all, GetPythonRules()...)
	all = append(all, GetTypeScriptRules()...)
	all = append(all, GetJavaRules()...)
	all = append(all, GetCPPRules()...)
	return all
}

// GetRulesByLanguage filters rules applicable to a specific programming language.
func GetRulesByLanguage(lang string) []RuleDefinition {
	upper := strings.ToUpper(strings.TrimSpace(lang))
	var matched []RuleDefinition
	for _, r := range GetAllCatalogRules() {
		if r.Language == "ALL" || r.Language == upper {
			matched = append(matched, r)
		}
	}
	return matched
}

// GetRulesByCategory filters rules by vulnerability category.
func GetRulesByCategory(category string) []RuleDefinition {
	upper := strings.ToUpper(strings.TrimSpace(category))
	var matched []RuleDefinition
	for _, r := range GetAllCatalogRules() {
		if strings.ToUpper(r.Category) == upper {
			matched = append(matched, r)
		}
	}
	return matched
}
