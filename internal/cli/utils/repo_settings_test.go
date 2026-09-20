// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"errors"
	"os"
	"testing"
)

func TestRepoSettingsDashboard(t *testing.T) {
	// Test Default URL
	os.Unsetenv("SCANDRIX_APP_URL")
	os.Unsetenv("APP_URL")
	url := GetScanDrixAppURL()
	if url != "https://app.scandrix.dev" {
		t.Fatalf("expected default https://app.scandrix.dev, got %s", url)
	}

	os.Setenv("SCANDRIX_APP_URL", "https://custom.scandrix.internal/")
	defer os.Unsetenv("SCANDRIX_APP_URL")
	if GetScanDrixAppURL() != "https://custom.scandrix.internal" {
		t.Fatalf("expected trimmed custom URL, got %s", GetScanDrixAppURL())
	}

	// Test section validation
	sec, err := ValidateRepositorySettingsSection("drixy-rules")
	if err != nil || sec != "drixy-rules" {
		t.Fatalf("expected drixy-rules to be valid, got %v, err: %v", sec, err)
	}

	_, err = ValidateRepositorySettingsSection("invalid-section")
	if err == nil {
		t.Fatalf("expected error for invalid section")
	}

	// Test label
	lbl := GetRepositorySettingsSectionLabel("drixy-rules")
	if lbl != "Drixy Rules" {
		t.Fatalf("expected Drixy Rules, got %s", lbl)
	}

	// Test URL builder
	dashURL := BuildRepositoryDashboardURL("https://app.scandrix.dev", "acme/repo", "drixy-rules")
	expected := "https://app.scandrix.dev/repositories/acme/repo?tab=drixy-rules"
	if dashURL != expected {
		t.Fatalf("expected %s, got %s", expected, dashURL)
	}
}

func TestRepoSettingsPatterns(t *testing.T) {
	field, err := ValidateRepositoryPatternField("ignore-files")
	if err != nil || field != "ignore-files" {
		t.Fatalf("expected ignore-files valid, got %s, err: %v", field, err)
	}

	_, err = ValidateRepositoryPatternField("unsupported-field")
	if err == nil {
		t.Fatalf("expected error for unsupported field")
	}

	initial := []string{"*.log", "vendor/**"}
	updated, added := AddPatternToStringSlice(initial, "*.tmp")
	if !added || len(updated) != 3 {
		t.Fatalf("expected added=true and len=3")
	}

	// Duplicate add
	updated2, added2 := AddPatternToStringSlice(updated, "*.tmp")
	if added2 || len(updated2) != 3 {
		t.Fatalf("expected duplicate to not be added")
	}

	// Remove
	removedSlice, removed := RemovePatternFromStringSlice(updated2, "*.log")
	if !removed || len(removedSlice) != 2 {
		t.Fatalf("expected pattern removed, len 2")
	}

	// Pattern matching
	pats := []string{"vendor/**", "*.min.js"}
	if !PatternMatchesPath(pats, "vendor/github.com/pkg/errors.go") {
		t.Fatalf("expected vendor match")
	}
	if !PatternMatchesPath(pats, "dist/bundle.min.js") {
		t.Fatalf("expected *.min.js match")
	}
	if PatternMatchesPath(pats, "src/main.go") {
		t.Fatalf("expected no match for src/main.go")
	}
}

func TestRepoSettingsSchema(t *testing.T) {
	err := ValidateRepositorySettingKey("review.enabled")
	if err != nil {
		t.Fatalf("expected review.enabled to be valid")
	}

	err = ValidateRepositorySettingKey("invalid.key")
	if err == nil {
		t.Fatalf("expected invalid key error")
	}

	b, err := ParseBooleanSetting("review.enabled", "true")
	if err != nil || !b {
		t.Fatalf("expected true, got %v", b)
	}

	b2, err := ParseBooleanSetting("review.enabled", "off")
	if err != nil || b2 {
		t.Fatalf("expected false, got %v", b2)
	}

	sev, err := ParseSeveritySetting("review.requestChanges.minSeverity", "critical")
	if err != nil || sev != "critical" {
		t.Fatalf("expected critical, got %v", sev)
	}

	pats := ParseRepositoryPatternList("*.log, *.tmp\nvendor/**\r\nnode_modules/**")
	if len(pats) != 4 {
		t.Fatalf("expected 4 patterns, got %d", len(pats))
	}
}

func TestCommandErrorsExt(t *testing.T) {
	diag := DiagnoseNetworkError(errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"), "http://127.0.0.1:8080")
	if !diag.IsNetworkError || diag.Category != "connection_refused" {
		t.Fatalf("expected connection_refused, got %+v", diag)
	}

	hints := BuildErrorRemedyHints(errors.New("unauthorized: auth required to perform review"))
	if len(hints) == 0 {
		t.Fatalf("expected hints for auth error")
	}
}

func TestFieldmaskExt(t *testing.T) {
	projector := NewMaskedProjector([]string{"user.name", "user.email", "status"})
	input := map[string]any{
		"user": map[string]any{
			"name":     "Alice",
			"email":    "alice@example.com",
			"password": "secret",
		},
		"status": "active",
		"internal_token": "token123",
	}

	res := projector.ProjectMap(input)
	if _, ok := res["internal_token"]; ok {
		t.Fatalf("internal_token should have been filtered out")
	}

	userMap, ok := res["user"].(map[string]any)
	if !ok {
		t.Fatalf("expected user map")
	}

	if _, ok := userMap["password"]; ok {
		t.Fatalf("password should have been filtered out")
	}

	if userMap["name"] != "Alice" || userMap["email"] != "alice@example.com" {
		t.Fatalf("expected name and email preserved")
	}
}
