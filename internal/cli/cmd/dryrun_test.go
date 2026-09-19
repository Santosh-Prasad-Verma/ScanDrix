// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"testing"
)

func TestDryRunCmd_Flags(t *testing.T) {
	cmd := dryRunCmd

	if cmd.Use != "dry-run [files...]" {
		t.Errorf("Unexpected Use: %s", cmd.Use)
	}

	flags := []string{"base-branch", "staged", "commit", "min-severity", "json", "agent"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("Expected flag --%s to exist", f)
		}
	}
}

func TestExecuteDryRun_EmptyDiff(t *testing.T) {
	err := ExecuteDryRun(context.Background(), DryRunOptions{
		MinSeverity: "low",
	})
	if err != nil {
		t.Fatalf("ExecuteDryRun failed: %v", err)
	}
}

func TestExecuteDryRun_InvalidSeverity(t *testing.T) {
	err := ExecuteDryRun(context.Background(), DryRunOptions{
		MinSeverity: "non-existent-severity",
	})
	// Should fail at config validation
	if err == nil {
		t.Fatal("Expected error for invalid severity, got nil")
	}
}
