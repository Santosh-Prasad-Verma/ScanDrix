// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package configcli

import (
	"testing"
)

func TestValidateRepositorySettingKey(t *testing.T) {
	validKeys := []string{
		"review.enabled",
		"review.autoApprove",
		"review.requestChanges.minSeverity",
		"patterns.ignoreFiles",
		"patterns.baseBranches",
		"patterns.ignoreTitles",
	}
	for _, k := range validKeys {
		if err := ValidateRepositorySettingKey(k); err != nil {
			t.Errorf("expected key %s to be valid, got %v", k, err)
		}
	}

	if err := ValidateRepositorySettingKey("invalid.key"); err == nil {
		t.Errorf("expected invalid.key to fail validation")
	}
}

func TestApplyRepositorySetting(t *testing.T) {
	settings := &RepositoryReviewSettings{}

	if err := ApplyRepositorySetting(settings, "review.enabled", "true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !settings.ReviewEnabled {
		t.Errorf("expected ReviewEnabled to be true")
	}

	if err := ApplyRepositorySetting(settings, "review.requestChanges.minSeverity", "high"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.RequestChangesMinSeverity != "high" {
		t.Errorf("expected RequestChangesMinSeverity to be high, got %s", settings.RequestChangesMinSeverity)
	}

	if err := ApplyRepositorySetting(settings, "review.requestChanges.minSeverity", "extreme"); err == nil {
		t.Errorf("expected extreme to be invalid severity")
	}

	if err := ApplyRepositorySetting(settings, "patterns.ignoreFiles", "dist/**, coverage/**"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(settings.IgnoredFilePatterns) != 2 {
		t.Fatalf("expected 2 ignored file patterns, got %v", settings.IgnoredFilePatterns)
	}
}

func TestAddAndRemoveRepositoryPattern(t *testing.T) {
	settings := &RepositoryReviewSettings{}

	if err := AddRepositoryPattern(settings, "base-branches", "release/*"); err != nil {
		t.Fatalf("AddRepositoryPattern failed: %v", err)
	}
	if len(settings.BaseBranchPatterns) != 1 || settings.BaseBranchPatterns[0] != "release/*" {
		t.Fatalf("expected release/*, got %v", settings.BaseBranchPatterns)
	}

	// Idempotent
	if err := AddRepositoryPattern(settings, "base-branches", "release/*"); err != nil {
		t.Fatalf("re-add failed: %v", err)
	}
	if len(settings.BaseBranchPatterns) != 1 {
		t.Fatalf("expected 1 pattern after duplicate add")
	}

	if err := RemoveRepositoryPattern(settings, "base-branches", "release/*"); err != nil {
		t.Fatalf("RemoveRepositoryPattern failed: %v", err)
	}
	if len(settings.BaseBranchPatterns) != 0 {
		t.Fatalf("expected 0 patterns after remove")
	}
}
