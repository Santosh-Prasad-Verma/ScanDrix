package usecases

import (
	"strings"
)

// PullRequestAuthorPolicy specifies the author inclusion policy filter.
type PullRequestAuthorPolicy string

const (
	AuthorPolicyAll        PullRequestAuthorPolicy = "all"
	AuthorPolicyReviewable PullRequestAuthorPolicy = "reviewable"
	AuthorPolicyIgnored    PullRequestAuthorPolicy = "ignored"
)

// AuthorPolicyConfigValue holds configured allowed and ignored user IDs or usernames.
type AuthorPolicyConfigValue struct {
	IgnoredUsers []string `json:"ignoredUsers,omitempty"`
	AllowedUsers []string `json:"allowedUsers,omitempty"`
}

// CompiledAuthorPolicyConfig caches lookup sets for fast author filtering.
type CompiledAuthorPolicyConfig struct {
	IgnoredUsers map[string]struct{}
	AllowedUsers map[string]struct{} // nil means unrestricted
}

// CompileAuthorPolicyConfig parses and normalizes author policy configuration.
func CompileAuthorPolicyConfig(configValue *AuthorPolicyConfigValue) CompiledAuthorPolicyConfig {
	ignored := make(map[string]struct{})
	if configValue != nil {
		for _, u := range configValue.IgnoredUsers {
			norm := strings.TrimSpace(strings.ToLower(u))
			if norm != "" {
				ignored[norm] = struct{}{}
			}
		}
	}

	var allowed map[string]struct{}
	if configValue != nil && len(configValue.AllowedUsers) > 0 {
		allowed = make(map[string]struct{})
		for _, u := range configValue.AllowedUsers {
			norm := strings.TrimSpace(strings.ToLower(u))
			if norm != "" {
				allowed[norm] = struct{}{}
			}
		}
		if len(allowed) == 0 {
			allowed = nil
		}
	}

	return CompiledAuthorPolicyConfig{
		IgnoredUsers: ignored,
		AllowedUsers: allowed,
	}
}

// IsAuthorExcludedByPolicy evaluates whether a specific author is excluded by the policy rules.
func IsAuthorExcludedByPolicy(author string, config CompiledAuthorPolicyConfig) bool {
	norm := strings.TrimSpace(strings.ToLower(author))
	if norm == "" {
		return false
	}

	if config.AllowedUsers != nil {
		if _, ok := config.AllowedUsers[norm]; !ok {
			return true
		}
	}

	if _, ok := config.IgnoredUsers[norm]; ok {
		return true
	}

	return false
}

// ShouldIncludeAuthorByPolicy applies the filter policy to determine inclusion in dashboard views.
func ShouldIncludeAuthorByPolicy(
	policy PullRequestAuthorPolicy,
	author string,
	config CompiledAuthorPolicyConfig,
) bool {
	if policy == AuthorPolicyAll || policy == "" {
		return true
	}

	excluded := IsAuthorExcludedByPolicy(author, config)
	if policy == AuthorPolicyReviewable {
		return !excluded
	}
	if policy == AuthorPolicyIgnored {
		return excluded
	}

	return true
}

// IntersectAssignedAndTeamScope calculates the effective repository list from RBAC and Team scopes.
// Nil assigned means unrestricted.
func IntersectAssignedAndTeamScope(
	assignedRepoIDs []string,
	teamRepoIDs []string,
) (effective []string, emptyScope bool) {
	// 1. Both present -> intersection (team ∩ assigned)
	if assignedRepoIDs != nil && teamRepoIDs != nil {
		assignedSet := make(map[string]struct{}, len(assignedRepoIDs))
		for _, id := range assignedRepoIDs {
			assignedSet[id] = struct{}{}
		}

		result := make([]string, 0)
		for _, id := range teamRepoIDs {
			if _, ok := assignedSet[id]; ok {
				result = append(result, id)
			}
		}

		if len(result) == 0 {
			return nil, true // Non-overlapping scopes mean caller sees 0 repos
		}
		return result, false
	}

	// 2. Only team scope
	if teamRepoIDs != nil {
		if len(teamRepoIDs) == 0 {
			return nil, true
		}
		return teamRepoIDs, false
	}

	// 3. Only assigned scope
	if assignedRepoIDs != nil {
		if len(assignedRepoIDs) == 0 {
			return nil, true
		}
		return assignedRepoIDs, false
	}

	// 4. Neither present -> unrestricted (org-wide)
	return nil, false
}
