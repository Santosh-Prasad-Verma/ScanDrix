// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/api"
)

func TestRepoConfigFormatters(t *testing.T) {
	// 1. FormatPatternList
	if FormatPatternList(nil) != "(none)" {
		t.Fatalf("expected (none), got %s", FormatPatternList(nil))
	}
	if FormatPatternList([]string{"*.go", "*.ts"}) != "*.go, *.ts" {
		t.Fatalf("unexpected pattern list: %s", FormatPatternList([]string{"*.go", "*.ts"}))
	}

	// 2. FormatSourceLabel
	src := &RepositorySettingSource{
		Level:           "team",
		OverriddenLevel: "organization",
	}
	label := FormatSourceLabel(src)
	if !strings.Contains(label, "team overrides organization") {
		t.Fatalf("unexpected source label: %s", label)
	}

	srcSingle := &RepositorySettingSource{
		Level: "repository",
	}
	if !strings.Contains(FormatSourceLabel(srcSingle), "repository") {
		t.Fatalf("unexpected source label: %s", FormatSourceLabel(srcSingle))
	}

	// 3. FormatRepositorySettings
	res := RepositorySettingsResult{
		RepositoryFullName: "scandrix/backend",
		Settings: DetailedRepoSettings{
			ReviewEnabled:             true,
			AutoApproveEnabled:        false,
			RequestChangesMinSeverity: "critical",
			IgnoredFilePatterns:       []string{"*.pb.go"},
			BaseBranchPatterns:         []string{"main"},
			IgnoredTitlePatterns:      []string{"^WIP"},
			Sources: map[string]RepositorySettingSource{
				"reviewEnabled": {Level: "repository"},
			},
		},
	}
	lines := FormatRepositorySettings(res)
	fullText := strings.Join(lines, "\n")
	if !strings.Contains(fullText, "scandrix/backend") || !strings.Contains(fullText, "Automated review:") {
		t.Fatalf("unexpected formatted settings output: %s", fullText)
	}

	// 4. FormatRepositorySetupPreview
	current := res.Settings
	next := current
	next.ReviewEnabled = false
	next.RequestChangesMinSeverity = "high"
	previewLines := FormatRepositorySetupPreview("scandrix/backend", current, next)
	previewText := strings.Join(previewLines, "\n")
	if !strings.Contains(previewText, "+ Automated review: true -> false") || !strings.Contains(previewText, "+ Min severity: critical -> high") {
		t.Fatalf("unexpected preview output: %s", previewText)
	}

	// 5. PrintRepoSettings & PrintRepoList
	buf := &bytes.Buffer{}
	PrintRepoSettings(buf, &api.RepositorySettings{
		Namespace:     "scandrix/core",
		DefaultBranch: "main",
		IgnoredPaths:  []string{"vendor/**"},
		FocusAreas:    []string{"security", "performance"},
		Reviewers:     []string{"alice", "bob"},
	})
	if !strings.Contains(buf.String(), "Repository: scandrix/core") || !strings.Contains(buf.String(), "vendor/**") {
		t.Fatalf("unexpected PrintRepoSettings output: %s", buf.String())
	}

	buf.Reset()
	PrintRepoList(buf, []api.TrackedRepository{
		{
			Namespace:     "scandrix/core",
			Provider:      "github",
			DefaultBranch: "main",
		},
	})
	if !strings.Contains(buf.String(), "Tracked Repositories (1):") {
		t.Fatalf("unexpected PrintRepoList output: %s", buf.String())
	}
}
