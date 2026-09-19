// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// SupportedRepoSettingsSections lists all valid tab/sections for repository settings.
var SupportedRepoSettingsSections = []string{
	"general",
	"review-categories",
	"custom-prompts",
	"suggestion-control",
	"pr-summary",
	"drixy-rules",
	"custom-messages",
	"business-rules",
}

var sectionLabels = map[string]string{
	"general":            "General",
	"review-categories":  "Review Categories",
	"custom-prompts":     "Custom Prompts",
	"suggestion-control": "Suggestion Control",
	"pr-summary":         "PR Summary",
	"drixy-rules":        "Drixy Rules",
	"custom-messages":    "Custom Messages",
	"business-rules":     "Business Rules",
}

// GetScanDrixAppURL resolves the dashboard web app URL from environment or defaults to the primary host.
func GetScanDrixAppURL() string {
	if val := strings.TrimSpace(os.Getenv("SCANDRIX_APP_URL")); val != "" {
		return strings.TrimRight(val, "/")
	}
	if val := strings.TrimSpace(os.Getenv("APP_URL")); val != "" {
		return strings.TrimRight(val, "/")
	}
	return "https://app.scandrix.dev"
}

// ValidateRepositorySettingsSection ensures the provided section is supported.
func ValidateRepositorySettingsSection(section string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(section))
	if norm == "" {
		return "general", nil
	}

	for _, s := range SupportedRepoSettingsSections {
		if norm == s {
			return norm, nil
		}
	}

	return "", NewCommandError(
		ErrCodeInvalidInput,
		fmt.Sprintf("Unsupported section '%s'. Supported sections: %s", section, strings.Join(SupportedRepoSettingsSections, ", ")),
	)
}

// GetRepositorySettingsSectionLabel returns the human-readable display label for a section.
func GetRepositorySettingsSectionLabel(section string) string {
	norm := strings.ToLower(strings.TrimSpace(section))
	if label, ok := sectionLabels[norm]; ok {
		return label
	}
	return "General"
}

// BuildRepositoryDashboardURL constructs a deep link to the web dashboard for a specific repository and tab.
func BuildRepositoryDashboardURL(appURL, repoNamespace, section string) string {
	base := strings.TrimRight(appURL, "/")
	if base == "" {
		base = GetScanDrixAppURL()
	}

	trimmedRepo := strings.Trim(repoNamespace, "/")
	if trimmedRepo == "" || trimmedRepo == "." {
		trimmedRepo = "local-workspace"
	}

	segments := strings.Split(trimmedRepo, "/")
	var escapedSegments []string
	for _, seg := range segments {
		escapedSegments = append(escapedSegments, url.PathEscape(seg))
	}
	dest := fmt.Sprintf("%s/repositories/%s", base, strings.Join(escapedSegments, "/"))

	if section != "" {
		validSection, err := ValidateRepositorySettingsSection(section)
		if err == nil && validSection != "general" {
			dest = fmt.Sprintf("%s?tab=%s", dest, url.QueryEscape(validSection))
		}
	}

	return dest
}
