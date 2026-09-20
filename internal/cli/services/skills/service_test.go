// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/skills"
)

func TestSkillsServiceSuite(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-skills-service-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srv := skills.DefaultService()

	// 1. Catalog check
	catalog := srv.Catalog()
	if len(catalog) < 10 {
		t.Fatalf("expected at least 10 skills in catalog, got %d", len(catalog))
	}

	// 2. Sync install
	syncRes, err := srv.Sync(tempDir, false, true)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if syncRes.Created == 0 {
		t.Fatalf("expected created skills > 0, got %d", syncRes.Created)
	}

	// Verify manifest file exists in cursor rules
	cursorManifest := filepath.Join(tempDir, ".cursor", "rules", ".scandrix-managed-skills.json")
	if _, err := os.Stat(cursorManifest); err != nil {
		t.Fatalf("expected manifest file %s to exist: %v", cursorManifest, err)
	}

	// 3. Check report
	report, err := srv.Check(tempDir)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if report.ActiveTargets == 0 {
		t.Fatalf("expected at least 1 active target, got %d", report.ActiveTargets)
	}
	if report.HealthyTargets == 0 {
		t.Fatalf("expected at least 1 healthy target after sync, got %d", report.HealthyTargets)
	}

	// 4. Uninstall
	unRes, err := srv.Uninstall(tempDir, false)
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	if unRes.Removed == 0 {
		t.Fatalf("expected removed skills > 0, got %d", unRes.Removed)
	}

	// Verify manifest file was removed
	if _, err := os.Stat(cursorManifest); !os.IsNotExist(err) {
		t.Fatalf("expected manifest file to be removed after uninstall")
	}
}
