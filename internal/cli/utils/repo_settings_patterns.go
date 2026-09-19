// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"strings"
)

// SupportedRepoPatternFields lists the valid pattern array fields.
var SupportedRepoPatternFields = []string{
	"ignore-files",
	"base-branches",
	"ignore-titles",
}

// ValidateRepositoryPatternField verifies that the pattern field name is valid.
func ValidateRepositoryPatternField(field string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(field))
	for _, f := range SupportedRepoPatternFields {
		if norm == f {
			return norm, nil
		}
	}
	return "", NewCommandError(
		ErrCodeInvalidInput,
		fmt.Sprintf("Unsupported pattern field '%s'. Supported pattern fields: %s", field, strings.Join(SupportedRepoPatternFields, ", ")),
	)
}

// AddPatternToStringSlice appends pattern to list if not already present, trimming whitespace.
func AddPatternToStringSlice(patterns []string, newPattern string) ([]string, bool) {
	trimmed := strings.TrimSpace(newPattern)
	if trimmed == "" {
		return patterns, false
	}

	for _, p := range patterns {
		if p == trimmed {
			return patterns, false
		}
	}

	result := make([]string, len(patterns), len(patterns)+1)
	copy(result, patterns)
	return append(result, trimmed), true
}

// RemovePatternFromStringSlice removes all instances of target pattern from slice.
func RemovePatternFromStringSlice(patterns []string, targetPattern string) ([]string, bool) {
	trimmed := strings.TrimSpace(targetPattern)
	if trimmed == "" {
		return patterns, false
	}

	var result []string
	removed := false
	for _, p := range patterns {
		if p == trimmed {
			removed = true
			continue
		}
		result = append(result, p)
	}

	if result == nil {
		result = []string{}
	}
	return result, removed
}

// PatternMatchesPath evaluates whether a file path matches any glob in patterns.
func PatternMatchesPath(patterns []string, path string) bool {
	normPath := strings.ReplaceAll(path, "\\", "/")
	for _, pat := range patterns {
		trimmed := strings.TrimSpace(pat)
		if trimmed == "" {
			continue
		}
		if matchGlob(trimmed, normPath) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, path string) bool {
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	if pattern == "**" || pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		sub := strings.TrimPrefix(pattern, "**/")
		if strings.HasSuffix(path, sub) || strings.Contains(path, "/"+sub) || path == sub {
			return true
		}
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			return true
		}
	}
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(path, suffix)
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return path == pattern || strings.Contains(path, pattern)
}
