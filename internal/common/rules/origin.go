// Package rules provides origin identification and classification for ScanDrix rules.
package rules

import "strings"

// RuleOrigin represents where a rule originated from.
type RuleOrigin string

const (
	OriginOrganization RuleOrigin = "organization"
	OriginRepository   RuleOrigin = "repository"
	OriginSystem       RuleOrigin = "system"
	OriginMarketplace  RuleOrigin = "marketplace"
)

// ResolveRuleOrigin determines the origin type from metadata or source path.
func ResolveRuleOrigin(sourcePath, explicitOrigin string) RuleOrigin {
	if explicitOrigin != "" {
		switch strings.ToLower(explicitOrigin) {
		case "organization", "org":
			return OriginOrganization
		case "repository", "repo":
			return OriginRepository
		case "system":
			return OriginSystem
		case "marketplace":
			return OriginMarketplace
		}
	}

	clean := strings.ToLower(filepathToSlash(sourcePath))
	if strings.Contains(clean, ".scandrix/rules/") || strings.Contains(clean, "rules/") {
		return OriginRepository
	}

	return OriginOrganization
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
