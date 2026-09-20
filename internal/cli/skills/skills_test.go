// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/skills"
)

func TestSkillsLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-skills-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. List bundled skills
	list := skills.ListBundledSkills()
	if len(list) < 8 {
		t.Fatalf("expected at least 8 bundled skills in catalog, got %d", len(list))
	}

	// 2. Install (dry run)
	dryRes, err := skills.Install(tempDir, true)
	if err != nil || dryRes.CreatedCount == 0 {
		t.Fatalf("dry run install failed: %v, %+v", err, dryRes)
	}

	// 3. Install (real)
	installRes, err := skills.Install(tempDir, false)
	if err != nil || installRes.CreatedCount == 0 {
		t.Fatalf("real install failed: %v, %+v", err, installRes)
	}

	// Verify file exists
	cursorRule := filepath.Join(tempDir, ".cursor", "rules", "scandrix-review.md")
	if _, err := os.Stat(cursorRule); err != nil {
		t.Fatalf("expected installed file %s to exist", cursorRule)
	}

	// 4. Test Prompt generation
	xmlPrompt := skills.GeneratePromptXML()
	if !strings.Contains(xmlPrompt, "<available_skills>") || !strings.Contains(xmlPrompt, "scandrix-review") {
		t.Fatalf("unexpected xml prompt output: %s", xmlPrompt)
	}

	jsonPrompt := skills.GeneratePromptJSON()
	if !strings.Contains(jsonPrompt, "scandrix-business-rules-validation") {
		t.Fatalf("unexpected json prompt output: %s", jsonPrompt)
	}

	// 5. Uninstall
	unRes, err := skills.Uninstall(tempDir, false)
	if err != nil || unRes.RemovedCount == 0 {
		t.Fatalf("uninstall failed: %v, %+v", err, unRes)
	}
}

func TestFormatSkillsPrompt(t *testing.T) {
	catalog := skills.BundledSkillsCatalog()
	if len(catalog) == 0 {
		t.Fatalf("catalog should not be empty")
	}

	// 1. XML output
	xmlOut, err := skills.FormatSkillsPrompt(catalog, "xml", nil)
	if err != nil {
		t.Fatalf("XML formatting failed: %v", err)
	}
	if !strings.HasPrefix(xmlOut, "<skills>") || !strings.HasSuffix(xmlOut, "</skills>") {
		t.Fatalf("unexpected XML root structure:\n%s", xmlOut)
	}
	if !strings.Contains(xmlOut, "scandrix-review") {
		t.Fatalf("XML output missing scandrix-review")
	}

	// 2. JSON output
	jsonOut, err := skills.FormatSkillsPrompt(catalog, "json", []string{"scandrix-review"})
	if err != nil {
		t.Fatalf("JSON formatting failed: %v", err)
	}
	if !strings.HasPrefix(jsonOut, "[\n") || !strings.Contains(jsonOut, `"name": "scandrix-review"`) {
		t.Fatalf("unexpected JSON output: %s", jsonOut)
	}

	// 3. Markdown output
	mdOut, err := skills.FormatSkillsPrompt(catalog, "markdown", []string{"scandrix-review"})
	if err != nil {
		t.Fatalf("Markdown formatting failed: %v", err)
	}
	if !strings.Contains(mdOut, "# ScanDrix Assistant Skills") || !strings.Contains(mdOut, "## 1. scandrix-review") {
		t.Fatalf("unexpected Markdown output: %s", mdOut)
	}

	// 4. Missing skill returns descriptive error
	_, errMissing := skills.FormatSkillsPrompt(catalog, "xml", []string{"nonexistent-custom-skill"})
	if errMissing == nil || !strings.Contains(errMissing.Error(), "not found") {
		t.Fatalf("expected error for nonexistent skill, got: %v", errMissing)
	}
}
