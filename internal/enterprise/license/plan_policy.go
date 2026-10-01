package license

import (
	"fmt"
	"strings"
)

// PlanQuota defines the resource limits and token rates associated with a subscription plan.
type PlanQuota struct {
	MonthlyTokens        int64 `json:"monthly_tokens"`
	BurstLimitPerMin     int64 `json:"burst_limit_per_min"`
	MaxSeats             int   `json:"max_seats"`
	MaxRepositories      int   `json:"max_repositories"`
	MaxConcurrentReviews int   `json:"max_concurrent_reviews"`
	BYOKAllowed          bool  `json:"byok_allowed"`
	PriorityQueueing     bool  `json:"priority_queueing"`
	AdvancedRulesAllowed bool  `json:"advanced_rules_allowed"`
}

// PlanQuota presets matching enterprise standards.
var (
	QuotaCommunity = PlanQuota{
		MonthlyTokens:        1_000_000, // 1M tokens / month (Free tier)
		BurstLimitPerMin:     100_000,   // 100K tokens / minute
		MaxSeats:             5,
		MaxRepositories:      5,
		MaxConcurrentReviews: 1,
		BYOKAllowed:          true, // Allowed when workspace supplies their own key
		PriorityQueueing:     false,
		AdvancedRulesAllowed: false,
	}

	QuotaDeveloper = PlanQuota{
		MonthlyTokens:        6_000_000, // 6M tokens / month (Developer tier)
		BurstLimitPerMin:     300_000,   // 300K tokens / minute
		MaxSeats:             10,
		MaxRepositories:      0, // 0 = unlimited
		MaxConcurrentReviews: 5,
		BYOKAllowed:          true,
		PriorityQueueing:     false,
		AdvancedRulesAllowed: true,
	}

	QuotaTeam = PlanQuota{
		MonthlyTokens:        14_000_000, // 14M tokens / month (Team tier)
		BurstLimitPerMin:     700_000,    // 700K tokens / minute
		MaxSeats:             25,
		MaxRepositories:      0, // 0 = unlimited
		MaxConcurrentReviews: 15,
		BYOKAllowed:          true,
		PriorityQueueing:     true,
		AdvancedRulesAllowed: true,
	}

	QuotaScale = PlanQuota{
		MonthlyTokens:        40_000_000, // 40M tokens / month (Scale tier)
		BurstLimitPerMin:     1_200_000,  // 1.2M tokens / minute
		MaxSeats:             100,
		MaxRepositories:      0, // 0 = unlimited
		MaxConcurrentReviews: 30,
		BYOKAllowed:          true,
		PriorityQueueing:     true,
		AdvancedRulesAllowed: true,
	}

	QuotaEnterprise = PlanQuota{
		MonthlyTokens:        100_000_000, // 100M tokens / month (custom SLA)
		BurstLimitPerMin:     2_000_000,   // 2M tokens / minute
		MaxSeats:             0,           // unlimited
		MaxRepositories:      0,           // unlimited
		MaxConcurrentReviews: 50,
		BYOKAllowed:          true,
		PriorityQueueing:     true,
		AdvancedRulesAllowed: true,
	}
)

// GetPlanQuota returns the resource quota assigned to a license tier.
func GetPlanQuota(tier LicenseTier) PlanQuota {
	switch NormalizeTier(tier) {
	case TierDeveloper:
		return QuotaDeveloper
	case TierTeam:
		return QuotaTeam
	case TierScale:
		return QuotaScale
	case TierEnterprise:
		return QuotaEnterprise
	default:
		return QuotaCommunity
	}
}

// NormalizeTier standardizes tier string representations (e.g. "PRO" -> "TEAM", "DEV" -> "DEVELOPER").
func NormalizeTier(tier LicenseTier) LicenseTier {
	upper := strings.ToUpper(strings.TrimSpace(string(tier)))
	switch upper {
	case "DEVELOPER", "DEV", "STARTER", "PLUS", "SPRINT":
		return TierDeveloper
	case "PRO", "TEAM", "TEAMS", "VELOCITY":
		return TierTeam
	case "SCALE":
		return TierScale
	case "ENTERPRISE", "ENT":
		return TierEnterprise
	default:
		return TierCommunity
	}
}

// Managed models authorized for Free / Community tier (when no customer BYOK is present).
var communityAllowedModels = map[string]bool{
	"gemini-2.5-flash-lite":                  true,
	"gemini-3.1-flash-lite":                  true,
	"gemini-2.5-flash":                       true,
	"minimax/minimax-m3:free":                true,
	"glm-5.3-flash":                          true,
	"zai/glm-5.3-flash":                      true,
	"thinkingmachines/inkling:free":          true,
	"nvidia/nemotron-3-ultra-550b-a55b:free": true,
	"free/deepseek-v4-pro-0813":              true,
	"free/gemini-3.7-flash":                  true,
	"free/gemini-3.1-pro":                    true,
	"free/gpt-5.6-luna":                      true,
	"free/deepseek-v4-flash-0731":            true,
	"free/glm-5.3-flash":                     true,
	"free/muse-spark-1.2":                    true,
}

// Frontier heavyweight models reserved for Team and Enterprise tiers (or BYOK).
var teamOnlyModels = map[string]bool{
	"claude-sonnet-5": true,
	"gpt-5.6-terra":   true,
	"grok-3":          true,
}

// Ultra-heavyweight models reserved exclusively for Enterprise tier (or BYOK).
var enterpriseOnlyModels = map[string]bool{
	"claude-opus-5":  true,
	"claude-fable-5": true,
	"gpt-5.6-sol":    true,
	"gpt-5.5":        true,
	"gpt-5.4":        true,
}

// CanAccessModel evaluates whether an organization is authorized to execute inference
// on a target model based on their plan tier and BYOK status.
//
// Policy rules:
//  1. BYOK Exception: If the customer provides their own provider API key (hasBYOK=true),
//     they fund the inference directly and can access ANY model supported by that provider.
//  2. Free / Community: Allowed only approved low-cost / trial models.
//  3. Developer: Allowed high-performance workhorse models (Gemini Flash, DeepSeek, GLM, Qwen, Kimi, Luna).
//  4. Team: Allowed all standard frontier workhorses (Claude Sonnet 5, GPT-5.6 Terra, Grok-3).
//  5. Enterprise: Full access to all models including ultra-flagships and private endpoints.
func CanAccessModel(tier LicenseTier, modelID string, hasBYOK bool) (bool, string) {
	cleanModel := strings.ToLower(strings.TrimSpace(modelID))

	// 1. BYOK always bypasses managed model restrictions
	if hasBYOK {
		return true, ""
	}

	normTier := NormalizeTier(tier)

	// 2. Community tier
	if normTier == TierCommunity {
		if communityAllowedModels[cleanModel] || strings.HasPrefix(cleanModel, "free/") {
			return true, ""
		}
		return false, fmt.Sprintf(
			"model '%s' is not available on the Free Community plan. Upgrade to Developer or Team to access advanced models, or configure a BYOK API key in workspace settings.",
			modelID,
		)
	}

	// 3. Developer tier
	if normTier == TierDeveloper {
		if teamOnlyModels[cleanModel] || enterpriseOnlyModels[cleanModel] {
			return false, fmt.Sprintf(
				"model '%s' requires a Team or Enterprise subscription (or custom BYOK credentials). Current plan: Developer.",
				modelID,
			)
		}
		return true, ""
	}

	// 4. Team tier
	if normTier == TierTeam {
		// Block ultra-expensive models on standard managed Team subscription
		if enterpriseOnlyModels[cleanModel] {
			return false, fmt.Sprintf(
				"model '%s' requires an Enterprise subscription or custom BYOK credentials. Current plan: Team.",
				modelID,
			)
		}
		return true, ""
	}

	// 5. Scale tier
	if normTier == TierScale {
		if cleanModel == "claude-opus-5" || cleanModel == "claude-fable-5" || cleanModel == "gpt-5.6-sol" {
			return false, fmt.Sprintf(
				"model '%s' requires an Enterprise subscription or custom BYOK credentials. Current plan: Scale.",
				modelID,
			)
		}
		return true, ""
	}

	// 6. Enterprise tier: all models allowed
	return true, ""
}

// GetAllocatedModelsList returns the list of managed model IDs available to a tier.
func GetAllocatedModelsList(tier LicenseTier) []string {
	normTier := NormalizeTier(tier)
	switch normTier {
	case TierEnterprise:
		return []string{
			"claude-opus-5", "claude-fable-5", "claude-sonnet-5",
			"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna",
			"gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.1-pro",
			"qwen3.8-max", "kimi-k3", "deepseek-chat", "grok-3", "mistral-large-3",
			"gemini-2.5-flash-lite", "gemini-3.1-flash-lite",
		}
	case TierScale:
		return []string{
			"claude-sonnet-5", "gpt-5.6-terra", "gpt-5.6-luna",
			"gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.1-pro",
			"qwen3.8-max", "kimi-k3", "deepseek-chat", "grok-3", "mistral-large-3",
			"gemini-2.5-flash-lite", "gemini-3.1-flash-lite",
		}
	case TierTeam:
		return []string{
			"claude-sonnet-5", "gpt-5.6-terra", "gpt-5.6-luna",
			"gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.1-pro",
			"qwen3.8-max", "kimi-k3", "deepseek-chat", "grok-3", "mistral-large-3",
			"gemini-2.5-flash-lite", "gemini-3.1-flash-lite",
		}
	case TierDeveloper:
		return []string{
			"gpt-5.6-luna",
			"gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash",
			"qwen3.8-max", "kimi-k3", "deepseek-chat", "mistral-large-3",
			"gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-3.1-flash-lite",
			"glm-5.3-flash", "minimax/minimax-m3:free",
		}
	default:
		return []string{
			"gemini-2.5-flash-lite",
			"gemini-3.1-flash-lite",
			"gemini-2.5-flash",
			"minimax/minimax-m3:free",
			"glm-5.3-flash",
			"thinkingmachines/inkling:free",
		}
	}
}
