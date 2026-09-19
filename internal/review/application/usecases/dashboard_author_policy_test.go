package usecases_test

import (
	"testing"

	"github.com/scandrix/backend/internal/review/application/usecases"
)

func TestAuthorPolicyFilter_CompileAndEvaluate(t *testing.T) {
	configVal := &usecases.AuthorPolicyConfigValue{
		IgnoredUsers: []string{"dependabot", "renovate[bot]", "github-actions"},
		AllowedUsers: []string{"alice", "bob", "charlie"},
	}

	compiled := usecases.CompileAuthorPolicyConfig(configVal)

	// 1. Policy "all" includes everyone
	if !usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyAll, "dependabot", compiled) {
		t.Fatal("expected policy 'all' to include dependabot")
	}
	if !usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyAll, "alice", compiled) {
		t.Fatal("expected policy 'all' to include alice")
	}
	if !usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyAll, "stranger", compiled) {
		t.Fatal("expected policy 'all' to include stranger")
	}

	// 2. Policy "reviewable" excludes ignored and unallowed users
	if usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyReviewable, "dependabot", compiled) {
		t.Fatal("expected policy 'reviewable' to exclude ignored user dependabot")
	}
	if usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyReviewable, "stranger", compiled) {
		t.Fatal("expected policy 'reviewable' to exclude unallowed user stranger")
	}
	if !usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyReviewable, "alice", compiled) {
		t.Fatal("expected policy 'reviewable' to include allowed user alice")
	}

	// 3. Policy "ignored" includes only excluded users
	if !usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyIgnored, "dependabot", compiled) {
		t.Fatal("expected policy 'ignored' to include dependabot")
	}
	if !usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyIgnored, "stranger", compiled) {
		t.Fatal("expected policy 'ignored' to include unallowed user stranger")
	}
	if usecases.ShouldIncludeAuthorByPolicy(usecases.AuthorPolicyIgnored, "alice", compiled) {
		t.Fatal("expected policy 'ignored' to exclude allowed user alice")
	}
}

func TestIntersectAssignedAndTeamScope(t *testing.T) {
	// 1. Both present with overlap
	assigned := []string{"repo1", "repo2", "repo3"}
	team := []string{"repo2", "repo3", "repo4"}
	effective, empty := usecases.IntersectAssignedAndTeamScope(assigned, team)
	if empty {
		t.Fatal("expected non-empty scope")
	}
	if len(effective) != 2 || effective[0] != "repo2" || effective[1] != "repo3" {
		t.Fatalf("expected [repo2, repo3], got %+v", effective)
	}

	// 2. Both present with NO overlap
	assigned = []string{"repo1"}
	team = []string{"repo2"}
	effective, empty = usecases.IntersectAssignedAndTeamScope(assigned, team)
	if !empty || effective != nil {
		t.Fatal("expected empty scope when no overlap")
	}

	// 3. Only team present
	effective, empty = usecases.IntersectAssignedAndTeamScope(nil, []string{"repoA", "repoB"})
	if empty || len(effective) != 2 {
		t.Fatalf("expected [repoA, repoB], got %+v", effective)
	}

	// 4. Only assigned present
	effective, empty = usecases.IntersectAssignedAndTeamScope([]string{"repoX"}, nil)
	if empty || len(effective) != 1 || effective[0] != "repoX" {
		t.Fatalf("expected [repoX], got %+v", effective)
	}

	// 5. Neither present (unrestricted)
	effective, empty = usecases.IntersectAssignedAndTeamScope(nil, nil)
	if empty || effective != nil {
		t.Fatal("expected unrestricted (nil, false)")
	}
}
