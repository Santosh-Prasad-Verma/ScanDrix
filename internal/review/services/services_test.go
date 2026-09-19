package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
)

func TestMessageTemplateProcessor(t *testing.T) {
	p := NewMessageTemplateProcessor()

	vars := domain.TemplateVariables{
		Author:        "alice",
		PRNumber:      42,
		RepoName:      "web-app",
		Summary:       "All tests passed and code reviewed.",
		FindingsCount: 3,
		FindingsList:  "- [CRITICAL] SQL injection in query.go",
		RulesChecked:  15,
		DurationSecs:  2.5,
		ErrorMessage:  "None",
	}

	tests := []struct {
		template string
		contains []string
	}{
		{
			template: "Review for {{ author }} on {{ repo_name }} #{{ pr_number }}: {{ summary }}",
			contains: []string{"Review for alice", "web-app #42", "All tests passed"},
		},
		{
			template: "Hey @author, found @findingsCount findings in @duration:\n@findings",
			contains: []string{"Hey alice", "found 3 findings", "in 2.50s", "[CRITICAL] SQL injection"},
		},
	}

	for _, tt := range tests {
		res := p.Process(tt.template, vars)
		for _, want := range tt.contains {
			if !strings.Contains(res, want) {
				t.Errorf("Process(%q) = %q; missing %q", tt.template, res, want)
			}
		}
	}
}

func TestFormatSuggestionComment(t *testing.T) {
	sugID := uuid.New()
	req := domain.LineCommentRequest{
		FilePath:   "internal/auth/jwt.go",
		LineNumber: 45,
		Body:       "Rotate the signing secret to an environment variable.",
		Suggestion: &domain.CodeSuggestion{
			ID:                 sugID,
			Severity:           domain.SeverityCritical,
			Category:           domain.CategorySecurity,
			OneSentenceSummary: "Exposed JWT Secret",
			ImprovedCode:       "secret := os.Getenv(\"JWT_SECRET\")",
			IsCommittable:      true,
			Label:              "SEC-001",
		},
		SuggestionCopyPrompt: true,
	}

	body := FormatSuggestionComment(req)

	if !strings.Contains(body, "🔴 CRITICAL") {
		t.Errorf("expected CRITICAL severity badge, got: %s", body)
	}
	if !strings.Contains(body, "```suggestion\nsecret := os.Getenv(\"JWT_SECRET\")\n```") {
		t.Errorf("expected committable suggestion block, got: %s", body)
	}
	if !strings.Contains(body, "<!-- scandrix-suggestion-id: "+sugID.String()+" -->") {
		t.Errorf("expected ScanDrix suggestion ID tag, got: %s", body)
	}
	if !strings.Contains(body, "<!-- scandrix-rule: SEC-001 -->") {
		t.Errorf("expected ScanDrix rule tag, got: %s", body)
	}
}

func TestSafeguardTriageService(t *testing.T) {
	safeguard := NewSafeguardTriageService()
	ctx := context.Background()

	patches := []*diff.FilePatch{
		{
			NewPath: "services/user.go",
			Hunks: []diff.Hunk{
				{NewStart: 10, NewLines: 20}, // lines 10-29
			},
		},
	}

	suggestions := []domain.CodeSuggestion{
		{
			ID:                 uuid.New(),
			RelevantFile:       "services/user.go",
			RelevantLinesStart: 15,
			RelevantLinesEnd:   20,
			SuggestionContent:  "Valid suggestion inside hunk",
			Category:           domain.CategoryBug,
		},
		{
			ID:                 uuid.New(),
			RelevantFile:       "services/user.go",
			RelevantLinesStart: 50,
			RelevantLinesEnd:   60,
			SuggestionContent:  "Out of bounds suggestion outside hunk",
			Category:           domain.CategoryBug,
		},
		{
			ID:                 uuid.New(),
			RelevantFile:       "services/nonexistent.go",
			RelevantLinesStart: 5,
			RelevantLinesEnd:   10,
			SuggestionContent:  "Suggestion on unedited file",
			Category:           domain.CategorySecurity,
		},
	}

	valid, discarded, err := safeguard.FilterSuggestions(ctx, suggestions, patches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(valid) != 1 {
		t.Errorf("expected 1 valid suggestion, got %d", len(valid))
	}
	if valid[0].RelevantLinesStart != 15 {
		t.Errorf("expected valid suggestion at line 15, got %d", valid[0].RelevantLinesStart)
	}
	if len(discarded) != 2 {
		t.Errorf("expected 2 discarded suggestions, got %d", len(discarded))
	}
}

func TestSandboxSyntaxValidator(t *testing.T) {
	validator := NewSandboxSyntaxValidator()
	ctx := context.Background()

	// 1. Valid Go code
	res, err := validator.ValidateSyntax(ctx, "go", "main.go", "package main\n\nfunc main() {}\n")
	if err != nil || !res.IsValid {
		t.Errorf("expected valid Go code, got: %+v, err: %v", res, err)
	}

	// 2. Invalid Go code
	res, err = validator.ValidateSyntax(ctx, "go", "main.go", "package main\n\nfunc broken( {\n")
	if err != nil || res.IsValid {
		t.Errorf("expected invalid Go code to fail, got: %+v", res)
	}

	// 3. Valid JSON
	res, err = validator.ValidateSyntax(ctx, "json", "config.json", `{"enabled": true, "count": 10}`)
	if err != nil || !res.IsValid {
		t.Errorf("expected valid JSON, got: %+v, err: %v", res, err)
	}

	// 4. Invalid JSON
	res, err = validator.ValidateSyntax(ctx, "json", "config.json", `{"enabled": true,}`)
	if err != nil || res.IsValid {
		t.Errorf("expected invalid JSON to fail, got: %+v", res)
	}

	// 5. Delimiter checks for Python / JS
	res, err = validator.ValidateSyntax(ctx, "python", "app.py", "def test():\n    return (1 + 2\n")
	if err != nil || res.IsValid {
		t.Errorf("expected unbalanced delimiter to fail, got: %+v", res)
	}
}

func TestSuggestionLLMValidator(t *testing.T) {
	syntax := NewSandboxSyntaxValidator()
	validator := NewSuggestionLLMValidator(syntax)
	ctx := context.Background()

	// 1. Too short description
	sugShort := domain.CodeSuggestion{
		SuggestionContent: "short",
	}
	res, _ := validator.ValidateSuggestion(ctx, sugShort, "")
	if res.Approved {
		t.Errorf("expected short suggestion to be rejected")
	}

	// 2. Cosmetic comment on bug
	sugComment := domain.CodeSuggestion{
		Category:          domain.CategoryBug,
		SuggestionContent: "Fix this potential nil pointer exception",
		ImprovedCode:      "// TODO: check nil",
	}
	res, _ = validator.ValidateSuggestion(ctx, sugComment, "")
	if res.Approved {
		t.Errorf("expected cosmetic comment for bug to be rejected")
	}

	// 3. Valid recommendation
	sugValid := domain.CodeSuggestion{
		Category:          domain.CategorySecurity,
		Severity:          domain.SeverityCritical,
		Language:          "go",
		RelevantFile:      "handler.go",
		SuggestionContent: "Validate authorization token before accessing tenant data",
		ExistingCode:      "return db.GetUser(id)",
		ImprovedCode:      "if !auth.IsAdmin(ctx) { return ErrForbidden }\nreturn db.GetUser(id)",
	}
	res, err := validator.ValidateSuggestion(ctx, sugValid, "")
	if err != nil || !res.Approved {
		t.Errorf("expected valid suggestion to be approved, got: %+v, err: %v", res, err)
	}
	if res.ConfidenceScore < 0.8 {
		t.Errorf("expected high confidence score, got %f", res.ConfidenceScore)
	}
}
