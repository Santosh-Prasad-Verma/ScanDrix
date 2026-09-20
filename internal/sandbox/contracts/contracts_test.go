package contracts_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/pkg/models"
)

func TestResolveRepoPath(t *testing.T) {
	repoDir := "/home/user/repo"

	validCases := []struct {
		input    string
		expected string
	}{
		{"src/main.go", "/home/user/repo/src/main.go"},
		{"README.md", "/home/user/repo/README.md"},
		{"nested/dir/file.txt", "/home/user/repo/nested/dir/file.txt"},
	}

	for _, tc := range validCases {
		res, err := contracts.ResolveRepoPath(repoDir, tc.input)
		if err != nil {
			t.Errorf("unexpected error for %q: %v", tc.input, err)
		}
		if res != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, res)
		}
	}

	invalidCases := []string{
		"/etc/passwd",
		"../../evil.sh",
		"../escape.txt",
		"nested/../../escape.txt",
		"foo/bar/../../../escape",
		"\\windows\\abs",
	}

	for _, p := range invalidCases {
		_, err := contracts.ResolveRepoPath(repoDir, p)
		if err == nil {
			t.Errorf("expected path traversal error for %q, got nil", p)
		}
	}
}

func TestBuildAuthHeader(t *testing.T) {
	// GitHub
	ghHeader, err := contracts.BuildAuthHeader(models.ProviderGitHub, "ghp_secret123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(ghHeader, "Authorization: Basic ") {
		t.Errorf("expected Basic auth for GitHub, got: %s", ghHeader)
	}

	// GitLab
	glHeader, err := contracts.BuildAuthHeader(models.ProviderGitLab, "glpat_secret123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(glHeader, "Authorization: Basic ") {
		t.Errorf("expected Basic auth for GitLab, got: %s", glHeader)
	}

	// Azure
	azHeader, err := contracts.BuildAuthHeader(models.ProviderAzure, "azpat_secret123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(azHeader, "Authorization: Basic ") {
		t.Errorf("expected Basic auth for Azure, got: %s", azHeader)
	}

	// Bitbucket with Atlassian API token
	bbHeader1, err := contracts.BuildAuthHeader(models.ProviderBitbucket, "ATATT_token123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(bbHeader1, "Authorization: Basic ") {
		t.Errorf("expected Basic auth for Bitbucket with ATATT, got: %s", bbHeader1)
	}

	// Bitbucket without username or ATATT should fail
	_, err = contracts.BuildAuthHeader(models.ProviderBitbucket, "classic_pwd", "")
	if err == nil {
		t.Errorf("expected error for Bitbucket without username, got nil")
	}

	// Bitbucket with username
	bbHeader2, err := contracts.BuildAuthHeader(models.ProviderBitbucket, "classic_pwd", "testuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(bbHeader2, "Authorization: Basic ") {
		t.Errorf("expected Basic auth for Bitbucket with username, got: %s", bbHeader2)
	}

	// Forgejo
	fjHeader, err := contracts.BuildAuthHeader(models.ProviderForgejo, "fj_token123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fjHeader != "Authorization: token fj_token123" {
		t.Errorf("expected token auth for Forgejo, got: %s", fjHeader)
	}

	// Empty token
	emptyHeader, err := contracts.BuildAuthHeader(models.ProviderGitHub, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if emptyHeader != "" {
		t.Errorf("expected empty header for empty token, got: %s", emptyHeader)
	}
}

func TestGetPRRefspec(t *testing.T) {
	cases := []struct {
		platform models.SCMProvider
		prNumber int
		cloneURL string
		branch   string
		expected string
	}{
		{models.ProviderGitHub, 42, "https://github.com/org/repo.git", "feat", "refs/pull/42/head"},
		{models.ProviderGitLab, 108, "https://gitlab.com/org/repo.git", "feat", "refs/merge-requests/108/head"},
		{models.ProviderBitbucket, 12, "https://bitbucket.org/org/repo.git", "feat", "refs/heads/feat"},
		{models.ProviderBitbucket, 12, "https://internal-bitbucket.corp.net/org/repo.git", "feat", "refs/pull-requests/12/from"},
		{models.ProviderAzure, 99, "https://dev.azure.com/org/proj/_git/repo", "feat", "refs/pull/99/merge"},
		{models.ProviderForgejo, 7, "https://codeberg.org/org/repo.git", "feat", "refs/pull/7/head"},
	}

	for _, tc := range cases {
		ref := contracts.GetPRRefspec(tc.platform, tc.prNumber, tc.cloneURL, tc.branch)
		if ref != tc.expected {
			t.Errorf("[%s] expected refspec %q, got %q", tc.platform, tc.expected, ref)
		}
	}
}

func TestBuildPrKey(t *testing.T) {
	validOrg := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

	// Valid PR Key
	key, err := contracts.BuildPrKey(validOrg, "repo-123", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11:repo-123:42"
	if key != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}

	// Invalid Org ID (not a UUID)
	_, err = contracts.BuildPrKey("not-a-uuid", "repo-123", 42)
	if err == nil {
		t.Fatal("expected error for non-UUID organizationId, got nil")
	}

	// Empty repository ID
	_, err = contracts.BuildPrKey(validOrg, "", 42)
	if err == nil {
		t.Fatal("expected error for empty repositoryId, got nil")
	}

	// Delimiter injection in repository ID
	_, err = contracts.BuildPrKey(validOrg, "evil:repo", 42)
	if err == nil {
		t.Fatal("expected error for delimiter injection in repositoryId, got nil")
	}

	// Delimiter injection in prNumber
	_, err = contracts.BuildPrKey(validOrg, "repo-123", "42:fake")
	if err == nil {
		t.Fatal("expected error for delimiter injection in prNumber, got nil")
	}
}

func TestAssertValidPrKey(t *testing.T) {
	validOrg := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

	// Valid 3-part
	if err := contracts.AssertValidPrKey(validOrg + ":my-repo:101"); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	// Valid 4-part CLI mode
	if err := contracts.AssertValidPrKey(validOrg + ":my-repo:cli:feature-x"); err != nil {
		t.Errorf("expected valid for CLI mode, got: %v", err)
	}

	// Valid 'trial'
	if err := contracts.AssertValidPrKey("trial:demo-repo:5"); err != nil {
		t.Errorf("expected valid for trial mode, got: %v", err)
	}

	// Invalid shape (2 parts or 5 parts)
	if err := contracts.AssertValidPrKey(validOrg + ":only-two"); err == nil {
		t.Error("expected error for 2-part key, got nil")
	}
	if err := contracts.AssertValidPrKey(validOrg + ":a:b:c:d"); err == nil {
		t.Error("expected error for 5-part key, got nil")
	}

	// Invalid first segment
	if err := contracts.AssertValidPrKey("invalid-org:my-repo:1"); err == nil {
		t.Error("expected error for invalid org segment, got nil")
	}

	// Missing repository
	if err := contracts.AssertValidPrKey(validOrg + ": :1"); err == nil {
		t.Error("expected error for empty repository segment, got nil")
	}

	// Missing prNumber
	if err := contracts.AssertValidPrKey(validOrg + ":my-repo: "); err == nil {
		t.Error("expected error for empty prNumber segment, got nil")
	}
}

func TestDecomposePrKey(t *testing.T) {
	validOrg := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

	// 3-part decompose
	dec, err := contracts.DecomposePrKey(validOrg + ":backend:250")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.OrganizationID != validOrg || dec.RepositoryID != "backend" || dec.PRNumber != "250" {
		t.Fatalf("unexpected decomposed fields: %+v", dec)
	}

	// 4-part decompose (CLI mode)
	decCLI, err := contracts.DecomposePrKey(validOrg + ":backend:cli:main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decCLI.OrganizationID != validOrg || decCLI.RepositoryID != "backend" || decCLI.PRNumber != "main" {
		t.Fatalf("unexpected decomposed fields for CLI mode: %+v", decCLI)
	}
}

