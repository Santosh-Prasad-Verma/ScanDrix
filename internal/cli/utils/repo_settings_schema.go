// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// SupportedRepoSettingKeys enumerates the canonical repository setting identifiers.
var SupportedRepoSettingKeys = []string{
	"review.enabled",
	"review.autoApprove",
	"review.requestChanges.minSeverity",
	"patterns.ignoreFiles",
	"patterns.baseBranches",
	"patterns.ignoreTitles",
}

// SupportedSeverities lists valid severity thresholds for automated change requests.
var SupportedSeverities = []string{
	"low",
	"medium",
	"high",
	"critical",
}

// ParseRepositoryPatternList splits comma-separated or newline-delimited strings into a clean slice.
func ParseRepositoryPatternList(value string) []string {
	clean := strings.ReplaceAll(value, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")
	var items []string

	lines := strings.Split(clean, "\n")
	for _, line := range lines {
		parts := strings.Split(line, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
	}
	return items
}

// ValidateRepositorySettingKey verifies if a configuration key is supported.
func ValidateRepositorySettingKey(key string) error {
	norm := strings.TrimSpace(key)
	for _, k := range SupportedRepoSettingKeys {
		if norm == k {
			return nil
		}
	}
	return NewCommandError(
		ErrCodeInvalidInput,
		fmt.Sprintf("Unsupported setting key '%s'. Supported keys: %s", key, strings.Join(SupportedRepoSettingKeys, ", ")),
	)
}

// ParseBooleanSetting parses true/false or 1/0 with clear error messaging.
func ParseBooleanSetting(key, value string) (bool, error) {
	norm := strings.ToLower(strings.TrimSpace(value))
	if norm == "true" || norm == "1" || norm == "yes" || norm == "on" {
		return true, nil
	}
	if norm == "false" || norm == "0" || norm == "no" || norm == "off" {
		return false, nil
	}
	return false, NewCommandError(
		ErrCodeInvalidInput,
		fmt.Sprintf("Setting '%s' expects 'true' or 'false', got '%s'", key, value),
	)
}

// ParseSeveritySetting validates and normalizes a severity level string.
func ParseSeveritySetting(key, value string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(value))
	for _, s := range SupportedSeverities {
		if norm == s {
			return norm, nil
		}
	}
	return "", NewCommandError(
		ErrCodeInvalidInput,
		fmt.Sprintf("Setting '%s' expects one of: %s (got '%s')", key, strings.Join(SupportedSeverities, ", "), value),
	)
}

// ParseIntegerSetting parses non-negative integer thresholds.
func ParseIntegerSetting(key, value string, min, max int) (int, error) {
	val, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, NewCommandError(
			ErrCodeInvalidInput,
			fmt.Sprintf("Setting '%s' expects an integer value: %v", key, err),
		)
	}
	if val < min || (max > 0 && val > max) {
		return 0, NewCommandError(
			ErrCodeInvalidInput,
			fmt.Sprintf("Setting '%s' must be between %d and %d", key, min, max),
		)
	}
	return val, nil
}
