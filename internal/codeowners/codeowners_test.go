package codeowners_test

import (
	"context"
	"sort"
	"testing"

	"github.com/scandrix/backend/internal/codeowners"
)

const sampleCodeowners = `# Global fallback
* @default-team

# Backend ownership
/internal/auth/** @security-team @alice
/internal/api/** @backend-team
*.go @go-reviewers

# Frontend ownership
/web/ @frontend-team
*.tsx @frontend-team

# Docs
/docs/ @docs-team
`

func TestParse(t *testing.T) {
	f := codeowners.Parse(sampleCodeowners)
	if len(f.Rules) != 7 {
		t.Fatalf("expected 7 rules, got %d", len(f.Rules))
	}

	if f.Rules[0].Pattern != "*" {
		t.Errorf("first rule pattern expected '*', got '%s'", f.Rules[0].Pattern)
	}
	if len(f.Rules[1].Owners) != 2 {
		t.Errorf("auth rule expected 2 owners, got %d", len(f.Rules[1].Owners))
	}
}

func TestMatchLastRuleWins(t *testing.T) {
	f := codeowners.Parse(sampleCodeowners)

	// A .go file in internal/auth should match both "*.go" and "/internal/auth/**"
	// Last matching rule wins — "*.go" is declared AFTER "/internal/auth/**", so *.go wins
	owners := f.Match("internal/auth/password_reset.go")
	if len(owners) == 0 {
		t.Fatal("expected owners for internal/auth/password_reset.go")
	}
	// *.go rule is the last matching one
	if owners[0] != "@go-reviewers" {
		t.Errorf("expected @go-reviewers (last match wins), got %v", owners)
	}
}

func TestMatchDirectory(t *testing.T) {
	f := codeowners.Parse(sampleCodeowners)

	owners := f.Match("docs/README.md")
	if len(owners) == 0 || owners[0] != "@docs-team" {
		t.Errorf("expected @docs-team for docs/README.md, got %v", owners)
	}
}

func TestMatchFallback(t *testing.T) {
	f := codeowners.Parse(sampleCodeowners)

	owners := f.Match("Makefile")
	// Makefile matches "* @default-team" only — no other pattern matches
	// But "*.go" rule doesn't match .Makefile
	if len(owners) == 0 {
		t.Fatal("expected fallback owners for Makefile")
	}
	if owners[0] != "@default-team" {
		t.Errorf("expected @default-team, got %v", owners)
	}
}

func TestMatchAllDedup(t *testing.T) {
	f := codeowners.Parse(sampleCodeowners)

	owners := f.MatchAll([]string{
		"internal/api/router.go",
		"internal/auth/auth.go",
		"docs/guide.md",
	})

	if len(owners) < 2 {
		t.Fatalf("expected multiple unique owners, got %v", owners)
	}
}

func TestReviewerAssignerExcludesAuthor(t *testing.T) {
	assigner := codeowners.NewReviewerAssigner(sampleCodeowners)

	assignment := assigner.AssignReviewers(context.Background(), []string{
		"internal/auth/oauth/service.go",
		"docs/setup.md",
	}, "go-reviewers")

	// @go-reviewers should be excluded since they're the PR author
	for _, r := range assignment.Reviewers {
		if r == "@go-reviewers" {
			t.Error("PR author @go-reviewers should be excluded from reviewers")
		}
	}

	// docs/setup.md should map to @docs-team (last matching rule for .md in docs is /docs/)
	// But *.go wins for .go files
	if len(assignment.FileMap) == 0 {
		t.Error("expected file map entries")
	}
}

func TestReviewerAssignerSorted(t *testing.T) {
	assigner := codeowners.NewReviewerAssigner(sampleCodeowners)

	assignment := assigner.AssignReviewers(context.Background(), []string{
		"internal/api/handler.go",
		"web/page.tsx",
		"docs/api.md",
	}, "nobody")

	if !sort.StringsAreSorted(assignment.Reviewers) {
		t.Errorf("reviewers should be sorted, got %v", assignment.Reviewers)
	}
}

type mockTeamResolver struct{}

func (m *mockTeamResolver) ResolveTeamMembers(ctx context.Context, teamSlug string) ([]string, error) {
	if teamSlug == "@frontend-team" || teamSlug == "frontend-team" {
		return []string{"@carol", "@dave"}, nil
	}
	return nil, nil
}

func TestReviewerAssignerWithTeamResolver(t *testing.T) {
	assigner := codeowners.NewReviewerAssigner(sampleCodeowners, codeowners.WithTeamResolver(&mockTeamResolver{}))

	assignment := assigner.AssignReviewers(context.Background(), []string{
		"web/page.tsx",
	}, "alice")

	if len(assignment.Reviewers) == 0 {
		t.Fatal("expected assigned reviewers")
	}
}
