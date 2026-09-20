// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_model_policy.go
// ═══════════════════════════════════════════════════════════════

package services

import "strings"

// ResolveDrixyRulesModelPolicy determines the optimal model based on plan tier, BYOK configuration, and task.
func ResolveDrixyRulesModelPolicy(planTier string, byokModel string) string {
	if byokModel != "" {
		return byokModel
	}
	switch strings.ToLower(planTier) {
	case "enterprise":
		return "claude-3-5-sonnet-20241022"
	case "team", "pro":
		return "gpt-4o"
	default:
		return "gpt-4o-mini"
	}
}
