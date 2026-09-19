// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

func TestParseCSVList(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"   ", nil},
		{"error,critical", []string{"error", "critical"}},
		{"  info ,  warning , high ", []string{"info", "warning", "high"}},
		{",,,", nil},
	}

	for _, tc := range cases {
		out := ParseCSVList(tc.input)
		if len(out) != len(tc.expected) {
			t.Errorf("input %q: expected length %d, got %d", tc.input, len(tc.expected), len(out))
			continue
		}
		for i, v := range tc.expected {
			if out[i] != v {
				t.Errorf("input %q at %d: expected %s, got %s", tc.input, i, v, out[i])
			}
		}
	}
}

func TestValidateSuggestionsOptions(t *testing.T) {
	// Valid options
	validOpts := SuggestionsOptions{
		PRURL:    "https://github.com/owner/repo/pull/42",
		Severity: []string{"error", "critical"},
		Category: []string{"security_vulnerability", "performance"},
	}
	if err := ValidateSuggestionsOptions(validOpts); err != nil {
		t.Fatalf("unexpected error for valid options: %v", err)
	}

	// Invalid URL
	invalidURLOpts := SuggestionsOptions{
		PRURL: "ftp://invalid-url.com",
	}
	if err := ValidateSuggestionsOptions(invalidURLOpts); err == nil {
		t.Errorf("expected error for non-http URL")
	}

	// Invalid Severity
	invalidSevOpts := SuggestionsOptions{
		Severity: []string{"extreme_hazard"},
	}
	if err := ValidateSuggestionsOptions(invalidSevOpts); err == nil {
		t.Errorf("expected error for invalid severity")
	}

	// Invalid Category
	invalidCatOpts := SuggestionsOptions{
		Category: []string{"non_existent_category"},
	}
	if err := ValidateSuggestionsOptions(invalidCatOpts); err == nil {
		t.Errorf("expected error for invalid category")
	}
}

func TestFormatSuggestionsOutput(t *testing.T) {
	res := &SuggestionsResult{
		TotalSuggestions: 2,
		FilteredCount:    2,
		Findings: []models.CodeFinding{
			{
				ID:        uuid.New(),
				FilePath:  "auth/login.go",
				StartLine: 10,
				Severity:  models.SeverityCritical,
				Category:  "security_vulnerability",
				Title:     "Plaintext Password Logging",
			},
			{
				ID:        uuid.New(),
				FilePath:  "db/conn.go",
				StartLine: 25,
				Severity:  models.SeverityHigh,
				Category:  "performance",
				Title:     "Unbounded Connection Pool",
			},
		},
	}

	// Terminal output
	termOut, err := FormatSuggestionsOutput(res, "terminal")
	if err != nil {
		t.Fatalf("unexpected terminal format error: %v", err)
	}
	if len(termOut) == 0 {
		t.Errorf("expected non-empty terminal output")
	}

	// JSON output
	jsonOut, err := FormatSuggestionsOutput(res, "json")
	if err != nil {
		t.Fatalf("unexpected json format error: %v", err)
	}
	if len(jsonOut) == 0 {
		t.Errorf("expected non-empty json output")
	}

	// Markdown output
	mdOut, err := FormatSuggestionsOutput(res, "markdown")
	if err != nil {
		t.Fatalf("unexpected markdown format error: %v", err)
	}
	if len(mdOut) == 0 {
		t.Errorf("expected non-empty markdown output")
	}
}

func TestPRCommandStructure(t *testing.T) {
	cmd := CreatePRCommand()
	if cmd.Use != "pr <number|url>" {
		t.Errorf("expected use 'pr <number|url>', got %s", cmd.Use)
	}

	var hasSuggestions, hasBusinessValidation bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "suggestions" {
			hasSuggestions = true
		}
		if sub.Name() == "business-validation" {
			hasBusinessValidation = true
		}
	}

	if !hasSuggestions {
		t.Errorf("missing 'suggestions' subcommand")
	}
	if !hasBusinessValidation {
		t.Errorf("missing 'business-validation' subcommand")
	}
}

func TestBusinessValidationOptionsValidation(t *testing.T) {
	// Missing both task-url and task-id
	opts := BusinessValidationOptions{}
	code, err := ExecuteBusinessValidationAction(context.Background(), nil, nil, opts)
	if err == nil {
		t.Errorf("expected error when neither task-url nor task-id is provided")
	}
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}
