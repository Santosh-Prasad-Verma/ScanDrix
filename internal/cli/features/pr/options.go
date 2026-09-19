// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var allowedSeverities = map[string]bool{
	"info":     true,
	"warning":  true,
	"error":    true,
	"critical": true,
	"high":     true,
	"medium":   true,
	"low":      true,
}

var allowedCategories = map[string]bool{
	"security_vulnerability": true,
	"performance":            true,
	"code_quality":           true,
	"best_practices":         true,
	"style":                  true,
	"bug":                    true,
	"complexity":             true,
	"maintainability":        true,
	"documentation":          true,
}

// SuggestionsOptions models CLI flags for `scandrix pr suggestions`.
type SuggestionsOptions struct {
	PRURL      string
	PRNumber   int
	RepoID     string
	Severity   []string
	Category   []string
	Fields     string
	Format     string
	Output     string
	Quiet      bool
	IsAgent    bool
	OutputFile string
}

// BusinessValidationOptions models CLI flags for `scandrix pr business-validation`.
type BusinessValidationOptions struct {
	Files      []string
	TaskURL    string
	TaskID     string
	Staged     bool
	Commit     string
	Branch     string
	DryRun     bool
	JSONOutput bool
	Quiet      bool
}

// ValidateSuggestionsOptions ensures parameters and filters are well-formed.
func ValidateSuggestionsOptions(opts SuggestionsOptions) error {
	if opts.PRURL != "" {
		parsed, err := url.Parse(opts.PRURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("invalid value for `--pr-url`: must be a valid HTTP/HTTPS URL")
		}
	}

	for _, s := range opts.Severity {
		norm := strings.ToLower(strings.TrimSpace(s))
		if !allowedSeverities[norm] {
			return fmt.Errorf("invalid severity %q: allowed values are info, warning, error, critical, high, medium, low", s)
		}
	}

	for _, c := range opts.Category {
		norm := strings.ToLower(strings.TrimSpace(c))
		if !allowedCategories[norm] {
			return fmt.Errorf("invalid category %q: allowed values are security_vulnerability, performance, code_quality, best_practices, style, bug, complexity, maintainability, documentation", c)
		}
	}

	return nil
}

// ParseCSVList splits a comma-separated string into non-empty trimmed tokens.
func ParseCSVList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		token := strings.TrimSpace(p)
		if token != "" {
			result = append(result, token)
		}
	}
	return result
}

// ParseOptionalInt safely parses an optional integer argument.
func ParseOptionalInt(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("must be a valid non-negative integer, got %q", raw)
	}
	return n, nil
}
