package application

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

func TestFullDownloadUseCase(t *testing.T) {
	mockSvc := &MockCentralizedConfigService{
		Entries: []domain.ConfigFileMeta{
			{
				Path:    "repo-1/scandrix-config.yaml",
				Content: "version: '1.2'\n",
			},
			{
				Path:    "repo-1/.drixy-rules/rule.yaml",
				Content: "title: Rule 1\n",
			},
		},
	}

	uc := NewFullDownloadUseCase(mockSvc)
	entries, err := uc.Execute(context.Background(), "org-1", "team-1", DownloadOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have global config + repo config + rule = 3 entries
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	hasGlobal := false
	for _, e := range entries {
		if e.Path == "scandrix-config.yaml" {
			hasGlobal = true
			if !strings.Contains(e.Content, "version: \"1.2\"") && !strings.Contains(e.Content, "version: '1.2'") {
				t.Errorf("global config missing version: %s", e.Content)
			}
		}
	}
	if !hasGlobal {
		t.Errorf("expected global scandrix-config.yaml to be synthesized")
	}
}

func TestDownloadHelpers(t *testing.T) {
	// Unique path test
	uc := &FullDownloadUseCase{}
	used := make(map[string]struct{})
	p1 := uc.getUniquePath("rules/sec.yaml", used)
	used[p1] = struct{}{}
	p2 := uc.getUniquePath("rules/sec.yaml", used)
	if p1 == p2 {
		t.Errorf("expected unique path, got collision: %s", p2)
	}
	if p2 != "rules/sec-1.yaml" {
		t.Errorf("unexpected collision path name: %s", p2)
	}

	// Diff custom messages
	parent := map[string]string{
		"general": "Parent rule",
		"pr":      "Review carefully",
	}
	child := map[string]string{
		"general": "Parent rule", // same as parent -> omit
		"pr":      "Modified PR message", // different -> keep
		"extra":   "New child message", // new -> keep
	}

	diff := DiffCustomMessages(parent, child)
	if len(diff) != 2 {
		t.Fatalf("expected 2 diff entries, got %d: %+v", len(diff), diff)
	}
	if diff["pr"] != "Modified PR message" || diff["extra"] != "New child message" {
		t.Errorf("unexpected diff entries: %+v", diff)
	}

	// Merge custom messages
	merged := MergeCustomMessages(parent, child)
	if merged["general"] != "Parent rule" || merged["pr"] != "Modified PR message" || merged["extra"] != "New child message" {
		t.Errorf("unexpected merge result: %+v", merged)
	}

	// Build config content with custom messages
	yamlWithMsgs, err := BuildConfigContentWithCustomMessages("version: '1.2'\n", diff)
	if err != nil {
		t.Fatalf("failed build config content: %v", err)
	}
	if !strings.Contains(yamlWithMsgs, "customMessages:") || !strings.Contains(yamlWithMsgs, "Modified PR message") {
		t.Errorf("missing custom messages in YAML: %s", yamlWithMsgs)
	}

	// Directory depth & sorting
	paths := []string{"src/api/v1/auth", "src", "src/api"}
	sorted := SortByDirectoryDepth(paths)
	if sorted[0] != "src" || sorted[1] != "src/api" || sorted[2] != "src/api/v1/auth" {
		t.Errorf("unexpected depth sort: %+v", sorted)
	}
}

func TestFullDownloadUseCase_Scenarios(t *testing.T) {
	mockSvc := &MockCentralizedConfigService{
		Entries: []domain.ConfigFileMeta{
			// Global review rule
			{
				Path:    ".drixy-rules/review/global-rule.yml",
				Content: "title: Global Rule\nscope: global\n",
			},
			// Global memory rule
			{
				Path:    ".drixy-rules/memories/global-memory.yml",
				Content: "title: Global Memory\nscope: global\n",
			},
			// Repo-level config
			{
				Path:    "payments-service/scandrix-config.yaml",
				Content: "version: '1.2'\nreviewMode: strict\n",
			},
			// Directory-group config
			{
				Path:    "payments-service/p-api-web/scandrix-config.yaml",
				Content: "version: '1.2'\nignorePaths:\n  - vendor/**\n",
			},
			// Directory-group review rule
			{
				Path:    "payments-service/p-api-web/.drixy-rules/review/auth-jwt.yml",
				Content: "title: Validate JWT\nseverity: critical\n",
			},
		},
	}

	uc := NewFullDownloadUseCase(mockSvc)
	entries, err := uc.Execute(context.Background(), "org-1", "team-1", DownloadOptions{
		OrganizationID:                   "org-1",
		MarkRulesAsPendingWithSourcePath: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 entries + 1 synthesized global scandrix-config.yaml = 6 entries
	if len(entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(entries))
	}

	pathSet := make(map[string]bool)
	for _, e := range entries {
		pathSet[e.Path] = true
	}

	expectedPaths := []string{
		"scandrix-config.yaml",
		".drixy-rules/review/global-rule.yml",
		".drixy-rules/memories/global-memory.yml",
		"payments-service/scandrix-config.yaml",
		"payments-service/p-api-web/scandrix-config.yaml",
		"payments-service/p-api-web/.drixy-rules/review/auth-jwt.yml",
	}

	for _, ep := range expectedPaths {
		if !pathSet[ep] {
			t.Errorf("missing expected path in download entries: %s", ep)
		}
	}
}

