package codeowners_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/codeowners"
)

type mockSCMFetcher struct {
	files map[string][]byte
}

func (m *mockSCMFetcher) GetFileContent(_ context.Context, _, _, path string) ([]byte, error) {
	if content, exists := m.files[path]; exists {
		return content, nil
	}
	return nil, errors.New("file not found: 404")
}

func TestFetchCODEOWNERSLocations(t *testing.T) {
	ctx := context.Background()

	// 1. Found in .github/CODEOWNERS
	fetcher1 := &mockSCMFetcher{
		files: map[string][]byte{
			".github/CODEOWNERS": []byte("* @backend-team\n"),
		},
	}
	content, loc, err := codeowners.Fetch(ctx, fetcher1, "org/repo", "main")
	if err != nil {
		t.Fatalf("expected to find CODEOWNERS in .github, got: %v", err)
	}
	if loc != ".github/CODEOWNERS" {
		t.Errorf("expected location .github/CODEOWNERS, got %s", loc)
	}
	if content != "* @backend-team\n" {
		t.Errorf("unexpected content: %s", content)
	}

	// 2. Found in root CODEOWNERS when .github is missing
	fetcher2 := &mockSCMFetcher{
		files: map[string][]byte{
			"CODEOWNERS": []byte("* @root-team\n"),
		},
	}
	content, loc, err = codeowners.Fetch(ctx, fetcher2, "org/repo", "main")
	if err != nil {
		t.Fatalf("expected to find root CODEOWNERS, got: %v", err)
	}
	if loc != "CODEOWNERS" {
		t.Errorf("expected location CODEOWNERS, got %s", loc)
	}
	if content != "* @root-team\n" {
		t.Errorf("unexpected content: %s", content)
	}

	// 3. Not found anywhere
	fetcher3 := &mockSCMFetcher{files: map[string][]byte{}}
	_, _, err = codeowners.Fetch(ctx, fetcher3, "org/repo", "main")
	if !errors.Is(err, codeowners.ErrCODEOWNERSNotFound) {
		t.Errorf("expected ErrCODEOWNERSNotFound, got: %v", err)
	}
}

func TestAutoAssignFromSCM(t *testing.T) {
	ctx := context.Background()
	fetcher := &mockSCMFetcher{
		files: map[string][]byte{
			".github/CODEOWNERS": []byte(`
*.go @go-team
/internal/auth/** @sec-ops @alice
/web/** @frontend-team
`),
		},
	}

	changedFiles := []string{
		"internal/auth/login.go",
		"web/App.tsx",
	}

	// Author is alice -> should exclude alice from reviewers
	assignment, err := codeowners.AutoAssignFromSCM(ctx, fetcher, "org/repo", "main", changedFiles, "alice")
	if err != nil {
		t.Fatalf("AutoAssignFromSCM failed: %v", err)
	}

	// Check reviewers
	hasSecOps := false
	hasFrontend := false
	hasAlice := false
	for _, r := range assignment.Reviewers {
		if r == "@sec-ops" {
			hasSecOps = true
		}
		if r == "@frontend-team" {
			hasFrontend = true
		}
		if r == "@alice" || r == "alice" {
			hasAlice = true
		}
	}

	if !hasSecOps {
		t.Errorf("expected @sec-ops in reviewers, got %v", assignment.Reviewers)
	}
	if !hasFrontend {
		t.Errorf("expected @frontend-team in reviewers, got %v", assignment.Reviewers)
	}
	if hasAlice {
		t.Errorf("PR author alice must be excluded, got %v", assignment.Reviewers)
	}
}

func TestAutoAssignFromSCMNotFound(t *testing.T) {
	ctx := context.Background()
	fetcher := &mockSCMFetcher{files: map[string][]byte{}}

	assignment, err := codeowners.AutoAssignFromSCM(ctx, fetcher, "org/repo", "main", []string{"file.go"}, "bob")
	if err != nil {
		t.Fatalf("expected nil error on missing CODEOWNERS, got: %v", err)
	}
	if len(assignment.Reviewers) != 0 {
		t.Errorf("expected 0 reviewers, got %d", len(assignment.Reviewers))
	}
}
