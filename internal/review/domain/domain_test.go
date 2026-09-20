package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

func TestCalculatePriorityScore(t *testing.T) {
	tests := []struct {
		severity ReviewSeverity
		category ReviewCategory
		minScore float64
	}{
		{SeverityCritical, CategorySecurity, 150.0},
		{SeverityCritical, CategoryBug, 130.0},
		{SeverityMajor, CategoryPerformance, 90.0},
		{SeverityMinor, CategoryArchitecture, 55.0},
		{SeverityInfo, CategoryRules, 25.0},
	}

	for _, tt := range tests {
		score := CalculatePriorityScore(tt.severity, tt.category)
		diff := score - tt.minScore
		if diff < -0.001 || diff > 0.001 {
			t.Errorf("CalculatePriorityScore(%s, %s) = %f; want %f", tt.severity, tt.category, score, tt.minScore)
		}
	}
}

func TestPrioritizeSuggestionsByScore(t *testing.T) {
	suggestions := []CodeSuggestion{
		{ID: uuid.New(), Severity: SeverityInfo, Category: CategoryRules},
		{ID: uuid.New(), Severity: SeverityCritical, Category: CategorySecurity},
		{ID: uuid.New(), Severity: SeverityMajor, Category: CategoryBug},
		{ID: uuid.New(), Severity: SeverityMinor, Category: CategoryPerformance},
	}

	result := PrioritizeSuggestionsByScore(suggestions, 2, nil)
	if len(result.Prioritized) != 2 {
		t.Fatalf("expected 2 prioritized suggestions, got %d", len(result.Prioritized))
	}
	if len(result.Discarded) != 2 {
		t.Fatalf("expected 2 discarded suggestions, got %d", len(result.Discarded))
	}

	// Highest score should be first (Critical Security)
	if result.Prioritized[0].Severity != SeverityCritical {
		t.Errorf("expected first prioritized to be CRITICAL, got %s", result.Prioritized[0].Severity)
	}
	// Second should be Major Bug
	if result.Prioritized[1].Severity != SeverityMajor {
		t.Errorf("expected second prioritized to be MAJOR, got %s", result.Prioritized[1].Severity)
	}
	// Discarded should have PriorityStatusDiscardedByQuantity
	if result.Discarded[0].PriorityStatus != PriorityStatusDiscardedByQuantity {
		t.Errorf("expected discarded reason to be discarded-by-quantity, got %s", result.Discarded[0].PriorityStatus)
	}
}

func TestFilterSuggestionsByDiffLines(t *testing.T) {
	suggestions := []CodeSuggestion{
		{
			ID:                 uuid.New(),
			RelevantFile:       "main.go",
			RelevantLinesStart: 10,
			RelevantLinesEnd:   15,
		},
		{
			ID:                 uuid.New(),
			RelevantFile:       "main.go",
			RelevantLinesStart: 50,
			RelevantLinesEnd:   55,
		},
		{
			ID:                 uuid.New(),
			RelevantFile:       "utils.go",
			RelevantLinesStart: 1,
			RelevantLinesEnd:   5,
		},
	}

	changedLines := map[string][]int{
		"main.go": {12, 13, 14}, // overlaps with first suggestion (10-15), not second (50-55)
	}

	valid, discarded := FilterSuggestionsByDiffLines(suggestions, changedLines)
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid suggestion, got %d", len(valid))
	}
	if valid[0].RelevantLinesStart != 10 {
		t.Errorf("expected valid suggestion at line 10, got %d", valid[0].RelevantLinesStart)
	}
	if len(discarded) != 2 {
		t.Fatalf("expected 2 discarded suggestions, got %d", len(discarded))
	}
	for _, d := range discarded {
		if d.PriorityStatus != PriorityStatusDiscardedByCodeDiff {
			t.Errorf("expected discard status discarded-by-code-diff, got %s", d.PriorityStatus)
		}
	}
}

func TestCodeReviewConfig_IsPathIgnored(t *testing.T) {
	cfg := DefaultCodeReviewConfig()

	tests := []struct {
		path    string
		ignored bool
	}{
		{"vendor/github.com/pkg/errors/errors.go", true},
		{"node_modules/react/index.js", true},
		{"src/app.min.js", true},
		{"src/components/Header.tsx", false},
		{"internal/service/user.go", false},
		{"go.sum", true},
		{"pnpm-lock.yaml", true},
	}

	for _, tt := range tests {
		got := cfg.IsPathIgnored(tt.path)
		if got != tt.ignored {
			t.Errorf("IsPathIgnored(%q) = %v; want %v", tt.path, got, tt.ignored)
		}
	}
}

func TestConvertFindingToSuggestion(t *testing.T) {
	finding := models.CodeFinding{
		ID:            uuid.New(),
		FilePath:      "pkg/auth/jwt.go",
		StartLine:     42,
		EndLine:       48,
		Severity:      models.SeverityCritical,
		Category:      "security",
		Title:         "Hardcoded JWT Secret",
		Description:   "Avoid hardcoding secrets",
		SuggestedDiff: "- secret = \"123\"\n+ secret = os.Getenv(\"JWT_SECRET\")",
		Fingerprint:   "rule_sec_01",
		CreatedAt:     time.Now().UTC(),
	}

	s := ConvertFindingToSuggestion(finding, "pr_123", 42)
	if s.RelevantFile != "pkg/auth/jwt.go" {
		t.Errorf("expected pkg/auth/jwt.go, got %s", s.RelevantFile)
	}
	if s.Severity != SeverityCritical {
		t.Errorf("expected CRITICAL, got %s", s.Severity)
	}
	if s.Category != CategorySecurity {
		t.Errorf("expected SECURITY, got %s", s.Category)
	}
	if !s.IsCommittable {
		t.Errorf("expected isCommittable to be true")
	}
	if s.RankScore <= 0 {
		t.Errorf("expected positive rank score, got %f", s.RankScore)
	}
}
