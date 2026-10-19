package grouping

import (
	"testing"
)

func TestBuildIndexedFileList(t *testing.T) {
	diffs := []DiffItem{
		{NewPath: "internal/auth/login.go", Additions: 15, Deletions: 2},
		{NewPath: "internal/auth/session.go", Additions: 30, Deletions: 5},
	}

	indexed := BuildIndexedFileList(diffs)
	expectedPrefix := "[0] internal/auth/login.go (+15, -2)\n[1] internal/auth/session.go (+30, -5)\n"
	if indexed != expectedPrefix {
		t.Fatalf("expected:\n%s\ngot:\n%s", expectedPrefix, indexed)
	}
}

func TestParseGroupingResponse(t *testing.T) {
	diffs := []DiffItem{
		{NewPath: "internal/auth/login.go"},
		{NewPath: "internal/auth/session.go"},
		{NewPath: "internal/db/schema.sql"},
		{NewPath: "docs/README.md"},
	}

	rawJSON := `[
		{"label": "Authentication Logic", "files": [0, 1]},
		{"label": "Database Changes", "files": [2]}
	]`

	groups, err := ParseGroupingResponse(rawJSON, diffs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 groups: Auth, DB, and unassigned README.md
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	if groups[0].Label != "Authentication Logic" || len(groups[0].Files) != 2 {
		t.Errorf("group 0 mismatch: %+v", groups[0])
	}

	if groups[1].Label != "Database Changes" || len(groups[1].Files) != 1 {
		t.Errorf("group 1 mismatch: %+v", groups[1])
	}

	// Unassigned file gets its own group
	if groups[2].Label != "docs/README.md" || len(groups[2].Files) != 1 {
		t.Errorf("group 2 mismatch: %+v", groups[2])
	}
}

func TestEstimateChangeSet(t *testing.T) {
	diffs := []DiffItem{
		{NewPath: "main.go", DiffText: "func main() {}\n+ // Added new handler\n"},
		{NewPath: "util.go", DiffText: "+ func Helper() {}\n"},
	}

	est := EstimateChangeSet(diffs)
	if est.Files != 2 {
		t.Errorf("expected 2 files, got %d", est.Files)
	}
	if est.TotalTokens <= 0 {
		t.Errorf("expected positive token projection, got %d", est.TotalTokens)
	}
	human := est.String()
	if human == "" {
		t.Errorf("expected non-empty string representation")
	}
}
