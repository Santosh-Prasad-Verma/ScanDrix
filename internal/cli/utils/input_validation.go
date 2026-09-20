// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParseOptionalNumber parses a positive integer from a string flag value.
// Returns nil if the raw string is empty.
// Returns a CommandError if the value is not a valid integer or <= 0.
func ParseOptionalNumber(raw, flag string) (*int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	val, err := strconv.Atoi(trimmed)
	if err != nil || val <= 0 {
		return nil, &CommandError{
			Code:     ErrCodeInvalidInput,
			Message:  fmt.Sprintf("Invalid %s value", flag),
			ExitCode: 1,
			Details: map[string]any{
				"flag": flag,
				"raw":  raw,
			},
		}
	}

	return &val, nil
}

// ParseCsvEnumList splits a comma-separated string, trims items, and validates
// each item against a case-insensitive list of allowed values.
func ParseCsvEnumList(raw, flag string, allowed []string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	var items []string
	for _, p := range parts {
		item := strings.TrimSpace(p)
		if item != "" {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return nil, nil
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[strings.ToLower(strings.TrimSpace(a))] = struct{}{}
	}

	var invalid []string
	for _, item := range items {
		if _, ok := allowedSet[strings.ToLower(item)]; !ok {
			invalid = append(invalid, item)
		}
	}

	if len(invalid) > 0 {
		return nil, &CommandError{
			Code:     ErrCodeInvalidInput,
			Message:  fmt.Sprintf("Invalid value for %s: %s", flag, strings.Join(invalid, ", ")),
			ExitCode: 1,
			Details: map[string]any{
				"flag":    flag,
				"invalid": invalid,
				"allowed": allowed,
			},
		}
	}

	return items, nil
}

// ValidateHTTPURL parses a raw string and ensures it is a valid http:// or https:// URL.
func ValidateHTTPURL(raw, flag string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", &CommandError{
			Code:     ErrCodeInvalidInput,
			Message:  fmt.Sprintf("Invalid %s value", flag),
			ExitCode: 1,
			Details: map[string]any{
				"flag": flag,
				"raw":  raw,
			},
		}
	}
	return trimmed, nil
}

// AssertStructuredOutputForFields ensures --fields is only used with --format json or --agent.
func AssertStructuredOutputForFields(fields, format string, isAgent bool) error {
	if strings.TrimSpace(fields) == "" {
		return nil
	}

	if isAgent || strings.ToLower(strings.TrimSpace(format)) == "json" {
		return nil
	}

	return &CommandError{
		Code:     ErrCodeInvalidInput,
		Message:  "--fields requires --format json or --agent",
		ExitCode: 1,
	}
}
