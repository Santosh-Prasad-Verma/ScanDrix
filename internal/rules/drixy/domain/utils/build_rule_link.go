// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: build_rule_link.go
// ═══════════════════════════════════════════════════════════════

package utils

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// DrixyRuleAppLinkTab defines the target tab in dashboard settings.
type DrixyRuleAppLinkTab string

const (
	DrixyRuleAppLinkTabMemories    DrixyRuleAppLinkTab = "memories"
	DrixyRuleAppLinkTabReviewRules DrixyRuleAppLinkTab = "review-rules"
)

// BuildDrixyRuleAppLinkParams contains options for constructing dashboard deep links.
type BuildDrixyRuleAppLinkParams struct {
	RepositoryID string
	RuleID       string
	TeamID       string
	Status       interfaces.DrixyRulesStatus
	Tab          DrixyRuleAppLinkTab
	BaseURL      string
}

// BuildDrixyRuleAppLink constructs the direct URL to the ScanDrix web dashboard for a given rule or memory.
func BuildDrixyRuleAppLink(params BuildDrixyRuleAppLinkParams) string {
	baseURL := params.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("APP_BASE_URL")
	}
	if baseURL == "" {
		baseURL = os.Getenv("API_USER_INVITE_BASE_URL")
	}
	if baseURL == "" {
		baseURL = os.Getenv("SCANDRIX_APP_BASE_URL")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return ""
	}

	scope := params.RepositoryID
	if scope == "" || scope == "global" {
		scope = "global"
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	q := parsed.Query()
	if params.Tab != "" {
		q.Set("tab", string(params.Tab))
	} else {
		q.Set("tab", string(DrixyRuleAppLinkTabReviewRules))
	}
	if params.TeamID != "" {
		q.Set("teamId", params.TeamID)
	}

	if params.Status == interfaces.DrixyRulesStatusPending || params.RuleID == "" {
		parsed.Path = fmt.Sprintf("/settings/code-review/%s/drixy-rules", scope)
	} else {
		parsed.Path = fmt.Sprintf("/settings/code-review/%s/drixy-rules/%s", scope, params.RuleID)
	}

	parsed.RawQuery = q.Encode()
	return parsed.String()
}
