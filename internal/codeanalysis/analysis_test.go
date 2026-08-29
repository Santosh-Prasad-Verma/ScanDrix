package codeanalysis_test

import (
	"testing"

	"github.com/scandrix/backend/internal/codeanalysis"
	"github.com/scandrix/backend/internal/codeanalysis/languages"
)

func TestLanguageDetection(t *testing.T) {
	cases := []struct {
		path     string
		content  string
		expected languages.Language
	}{
		{"main.go", "", languages.LangGo},
		{"index.tsx", "", languages.LangTypeScript},
		{"service.py", "", languages.LangPython},
		{"scripts/deploy.sh", "#!/usr/bin/env python3\nprint('hi')", languages.LangPython},
		{"Dockerfile", "", languages.LangDockerfile},
		{"terraform/main.tf", "", languages.LangTerraform},
	}

	for _, c := range cases {
		lang := languages.DetectLanguage(c.path, c.content)
		if lang != c.expected {
			t.Errorf("path %s: expected %s, got %s", c.path, c.expected, lang)
		}
	}
}

func TestGoASTAnalysis(t *testing.T) {
	code := `package auth

import "context"

type UserRepo struct{}

func (r *UserRepo) Authenticate(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	return true, nil
}
`
	analysis, err := languages.AnalyzeGoSource("auth.go", code)
	if err != nil {
		t.Fatalf("AnalyzeGoSource failed: %v", err)
	}

	if analysis.PackageName != "auth" {
		t.Errorf("expected package auth, got %s", analysis.PackageName)
	}

	if len(analysis.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(analysis.Functions))
	}

	fn := analysis.Functions[0]
	if fn.Name != "Authenticate" {
		t.Errorf("expected function name Authenticate, got %s", fn.Name)
	}

	enclosing := analysis.FindEnclosingFunction(8)
	if enclosing == nil || enclosing.Name != "Authenticate" {
		t.Errorf("expected line 8 to resolve to Authenticate, got: %+v", enclosing)
	}
}

func TestSuppressionFilter(t *testing.T) {
	lines := []string{
		"package main",
		"// @scandrix-ignore SECURITY_SECRET",
		"const key = \"AKIAIOSFODNN7EXAMPLE\"",
		"var x = 10 // @scandrix-disable",
	}

	filter := codeanalysis.ParseSuppressions(lines)

	if !filter.IsSuppressed(3, "SECURITY_SECRET") {
		t.Error("expected line 3 to suppress SECURITY_SECRET")
	}

	if filter.IsSuppressed(3, "OTHER_RULE") {
		t.Error("expected line 3 not to suppress OTHER_RULE")
	}

	if !filter.IsSuppressed(4, "ANY_RULE") {
		t.Error("expected line 4 to suppress all rules")
	}
}

func TestIgnoreMatcher(t *testing.T) {
	matcher := codeanalysis.NewIgnoreMatcher([]string{"*.generated.ts"})

	if !matcher.ShouldIgnore("package-lock.json") {
		t.Error("expected package-lock.json to be ignored")
	}
	if !matcher.ShouldIgnore("vendor/github.com/foo/bar.go") {
		t.Error("expected vendor to be ignored")
	}
	if !matcher.ShouldIgnore("src/types.generated.ts") {
		t.Error("expected *.generated.ts to be ignored")
	}
	if matcher.ShouldIgnore("cmd/api/main.go") {
		t.Error("expected cmd/api/main.go not to be ignored")
	}
}
