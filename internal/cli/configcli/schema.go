// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package configcli

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/cli/utils"
)

// SupportedRepoSettingKeys defines allowable keys for 'config set [repository] <key> <value>'.
var SupportedRepoSettingKeys = []string{
	"review.enabled",
	"review.autoApprove",
	"review.requestChanges.minSeverity",
	"patterns.ignoreFiles",
	"patterns.baseBranches",
	"patterns.ignoreTitles",
}

// SupportedRepoPatternFields defines valid pattern collections.
var SupportedRepoPatternFields = []string{
	"ignore-files",
	"base-branches",
	"ignore-titles",
}

// ValidateRepositorySettingKey ensures key is in SupportedRepoSettingKeys.
func ValidateRepositorySettingKey(key string) error {
	for _, k := range SupportedRepoSettingKeys {
		if k == key {
			return nil
		}
	}
	return &utils.CommandError{
		Code:     utils.ErrCodeInvalidInput,
		Message:  fmt.Sprintf("Unsupported setting key '%s'. Supported keys: %s", key, strings.Join(SupportedRepoSettingKeys, ", ")),
		ExitCode: 1,
	}
}

// ValidateRepositoryPatternField ensures field is supported and returns normalized field.
func ValidateRepositoryPatternField(field string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(field))
	for _, f := range SupportedRepoPatternFields {
		if f == norm {
			return norm, nil
		}
	}
	// Aliases
	if norm == "ignore" || norm == "ignored_paths" || norm == "ignore-file" {
		return "ignore-files", nil
	}
	if norm == "base-branch" || norm == "branches" {
		return "base-branches", nil
	}
	if norm == "ignore-title" || norm == "titles" {
		return "ignore-titles", nil
	}

	return "", &utils.CommandError{
		Code:     utils.ErrCodeInvalidInput,
		Message:  fmt.Sprintf("Unsupported pattern field '%s'. Supported pattern fields: %s", field, strings.Join(SupportedRepoPatternFields, ", ")),
		ExitCode: 1,
	}
}

// ParseRepositoryPatternList splits comma- or newline-separated values.
func ParseRepositoryPatternList(value string) []string {
	var results []string
	lines := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == ','
	})
	seen := make(map[string]struct{})
	for _, item := range lines {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			if _, exists := seen[trimmed]; !exists {
				seen[trimmed] = struct{}{}
				results = append(results, trimmed)
			}
		}
	}
	return results
}

func parseBooleanSetting(key, value string) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "true" || v == "1" || v == "yes" {
		return true, nil
	}
	if v == "false" || v == "0" || v == "no" {
		return false, nil
	}
	return false, &utils.CommandError{
		Code:     utils.ErrCodeInvalidInput,
		Message:  fmt.Sprintf("Setting '%s' expects 'true' or 'false'.", key),
		ExitCode: 1,
	}
}

// ApplyRepositorySetting validates and mutates RepositoryReviewSettings with a key-value pair.
func ApplyRepositorySetting(settings *RepositoryReviewSettings, key, value string) error {
	if err := ValidateRepositorySettingKey(key); err != nil {
		return err
	}

	switch key {
	case "review.enabled":
		b, err := parseBooleanSetting(key, value)
		if err != nil {
			return err
		}
		settings.ReviewEnabled = b

	case "review.autoApprove":
		b, err := parseBooleanSetting(key, value)
		if err != nil {
			return err
		}
		settings.AutoApproveEnabled = b

	case "review.requestChanges.minSeverity":
		v := strings.ToLower(strings.TrimSpace(value))
		if v != "low" && v != "medium" && v != "high" && v != "critical" {
			return &utils.CommandError{
				Code:     utils.ErrCodeInvalidInput,
				Message:  fmt.Sprintf("Setting '%s' expects one of: low, medium, high, critical.", key),
				ExitCode: 1,
			}
		}
		settings.RequestChangesMinSeverity = v

	case "patterns.ignoreFiles":
		patterns := ParseRepositoryPatternList(value)
		settings.IgnoredFilePatterns = patterns
		settings.IgnoredPaths = patterns

	case "patterns.baseBranches":
		settings.BaseBranchPatterns = ParseRepositoryPatternList(value)

	case "patterns.ignoreTitles":
		settings.IgnoredTitlePatterns = ParseRepositoryPatternList(value)

	default:
		return &utils.CommandError{
			Code:     utils.ErrCodeInvalidInput,
			Message:  fmt.Sprintf("Unsupported setting key '%s'.", key),
			ExitCode: 1,
		}
	}

	return nil
}

// AddRepositoryPattern adds pattern to the respective field list in settings.
func AddRepositoryPattern(settings *RepositoryReviewSettings, field, pattern string) error {
	normField, err := ValidateRepositoryPatternField(field)
	if err != nil {
		return err
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return &utils.CommandError{
			Code:     utils.ErrCodeInvalidInput,
			Message:  "Pattern value cannot be empty.",
			ExitCode: 1,
		}
	}

	switch normField {
	case "ignore-files":
		for _, p := range settings.IgnoredFilePatterns {
			if p == pattern {
				return nil
			}
		}
		settings.IgnoredFilePatterns = append(settings.IgnoredFilePatterns, pattern)
		settings.IgnoredPaths = append(settings.IgnoredPaths, pattern)

	case "base-branches":
		for _, p := range settings.BaseBranchPatterns {
			if p == pattern {
				return nil
			}
		}
		settings.BaseBranchPatterns = append(settings.BaseBranchPatterns, pattern)

	case "ignore-titles":
		for _, p := range settings.IgnoredTitlePatterns {
			if p == pattern {
				return nil
			}
		}
		settings.IgnoredTitlePatterns = append(settings.IgnoredTitlePatterns, pattern)
	}

	return nil
}

// RemoveRepositoryPattern removes pattern from the respective field list in settings.
func RemoveRepositoryPattern(settings *RepositoryReviewSettings, field, pattern string) error {
	normField, err := ValidateRepositoryPatternField(field)
	if err != nil {
		return err
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return &utils.CommandError{
			Code:     utils.ErrCodeInvalidInput,
			Message:  "Pattern value cannot be empty.",
			ExitCode: 1,
		}
	}

	switch normField {
	case "ignore-files":
		var updatedFiles []string
		for _, p := range settings.IgnoredFilePatterns {
			if p != pattern {
				updatedFiles = append(updatedFiles, p)
			}
		}
		settings.IgnoredFilePatterns = updatedFiles

		var updatedPaths []string
		for _, p := range settings.IgnoredPaths {
			if p != pattern {
				updatedPaths = append(updatedPaths, p)
			}
		}
		settings.IgnoredPaths = updatedPaths

	case "base-branches":
		var updated []string
		for _, p := range settings.BaseBranchPatterns {
			if p != pattern {
				updated = append(updated, p)
			}
		}
		settings.BaseBranchPatterns = updated

	case "ignore-titles":
		var updated []string
		for _, p := range settings.IgnoredTitlePatterns {
			if p != pattern {
				updated = append(updated, p)
			}
		}
		settings.IgnoredTitlePatterns = updated
	}

	return nil
}
