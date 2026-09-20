// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: enrich_rules_with_context_references.go
// ═══════════════════════════════════════════════════════════════

package utils

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// EnrichRulesWithContextReferences populates external reference attributes.
func EnrichRulesWithContextReferences(
	ctx context.Context,
	rules []interfaces.DrixyRule,
	loader *services.ExternalReferenceLoaderService,
) []interfaces.DrixyRule {
	if loader == nil {
		return rules
	}
	out := make([]interfaces.DrixyRule, len(rules))
	for i, r := range rules {
		copied := r
		loader.EnrichRule(ctx, &copied)
		out[i] = copied
	}
	return out
}
