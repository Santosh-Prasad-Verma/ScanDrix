// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package configcli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWizard_NonInteractiveDefaults(t *testing.T) {
	current := RepositoryReviewSettings{
		Namespace: "scandrix/test-repo",
	}

	opts := WizardOptions{
		Yes: true,
	}

	res, err := RunWizard(current, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.EqualFold(res.RequestChangesMinSeverity, "high") {
		t.Errorf("expected default severity 'high', got %q", res.RequestChangesMinSeverity)
	}

	if len(res.IgnoredFilePatterns) != len(RecommendedIgnoredFilePatterns) {
		t.Errorf("expected %d recommended ignored file patterns, got %d", len(RecommendedIgnoredFilePatterns), len(res.IgnoredFilePatterns))
	}
}

func TestWizard_InteractiveSimulation(t *testing.T) {
	current := RepositoryReviewSettings{
		Namespace: "scandrix/core",
	}

	// Simulated user input:
	// General:
	//   Automated code review -> "y"
	//   Auto-approve clean PRs -> "n"
	//   Minimum severity -> "medium"
	// Patterns:
	//   Ignored files -> "1" (preset recommendation)
	//   Base branches -> "1" (preset recommendation)
	//   Ignored PR titles -> "1" (preset recommendation)
	// Confirmation:
	//   Next action -> "apply"
	inputLines := []string{
		"y",
		"n",
		"medium",
		"1",
		"1",
		"1",
		"apply",
	}
	input := strings.Join(inputLines, "\n") + "\n"

	var out bytes.Buffer
	opts := WizardOptions{
		In:  strings.NewReader(input),
		Out: &out,
	}

	res, err := RunWizard(current, opts)
	if err != nil {
		t.Fatalf("unexpected error in interactive simulation: %v", err)
	}

	if !res.ReviewEnabled {
		t.Errorf("expected ReviewEnabled to be true")
	}
	if res.AutoApproveEnabled {
		t.Errorf("expected AutoApproveEnabled to be false")
	}
	if res.RequestChangesMinSeverity != "medium" {
		t.Errorf("expected severity 'medium', got %q", res.RequestChangesMinSeverity)
	}
	if len(res.IgnoredFilePatterns) == 0 {
		t.Errorf("expected IgnoredFilePatterns to be populated")
	}
	if len(res.BaseBranchPatterns) == 0 {
		t.Errorf("expected BaseBranchPatterns to be populated")
	}
}

func TestParseCommaPatterns(t *testing.T) {
	raw := "  yarn.lock , package-lock.json,  dist/** , "
	patterns := parseCommaPatterns(raw)

	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns, got %d", len(patterns))
	}
	if patterns[0] != "yarn.lock" || patterns[1] != "package-lock.json" || patterns[2] != "dist/**" {
		t.Errorf("unexpected patterns: %v", patterns)
	}
}
