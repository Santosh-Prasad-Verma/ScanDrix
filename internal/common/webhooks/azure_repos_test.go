package webhooks

import (
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestResolveGitLabDraftStatus(t *testing.T) {
	if ResolveGitLabDraftStatus(nil) {
		t.Errorf("expected false for nil")
	}

	mrDraft := &GitLabDraftLike{Draft: boolPtr(true)}
	if !ResolveGitLabDraftStatus(mrDraft) {
		t.Errorf("expected true for draft: true")
	}

	mrWip := &GitLabDraftLike{WorkInProgress: boolPtr(true)}
	if !ResolveGitLabDraftStatus(mrWip) {
		t.Errorf("expected true for work_in_progress: true")
	}

	mrFalse := &GitLabDraftLike{Draft: boolPtr(false), WorkInProgress: boolPtr(false)}
	if ResolveGitLabDraftStatus(mrFalse) {
		t.Errorf("expected false for draft: false")
	}
}

func TestIsGitLabDraftToReadyChange(t *testing.T) {
	if IsGitLabDraftToReadyChange(nil) {
		t.Errorf("expected false for nil")
	}

	// 1. Direct transition from Draft: true -> false
	ch1 := &GitLabDraftChangesLike{
		Draft: &GitLabDraftChangeLike{
			Previous: boolPtr(true),
			Current:  boolPtr(false),
		},
	}
	if !IsGitLabDraftToReadyChange(ch1) {
		t.Errorf("expected true for draft: true -> false")
	}

	// 2. Draft unchanged
	ch2 := &GitLabDraftChangesLike{
		Draft: &GitLabDraftChangeLike{
			Previous: boolPtr(false),
			Current:  boolPtr(true),
		},
	}
	if IsGitLabDraftToReadyChange(ch2) {
		t.Errorf("expected false for draft: false -> true")
	}

	// 3. WIP transition fallback
	ch3 := &GitLabDraftChangesLike{
		WorkInProgress: &GitLabDraftChangeLike{
			Previous: boolPtr(true),
			Current:  boolPtr(false),
		},
	}
	if !IsGitLabDraftToReadyChange(ch3) {
		t.Errorf("expected true for wip: true -> false")
	}
}

func TestAzureReposMappedPlatform(t *testing.T) {
	adapter := &AzureReposMappedPlatform{}

	payload := AzureReposWebhookPayload{
		EventType: "git.pullrequest.created",
	}
	payload.Resource.PullRequestID = 101
	payload.Resource.Title = "Fix memory leak"
	payload.Resource.Description = "Ensures buffers are freed"
	payload.Resource.URL = "https://dev.azure.com/scandrix/project/_git/backend/pullrequest/101"
	payload.Resource.SourceRefName = "refs/heads/feature"
	payload.Resource.TargetRefName = "refs/heads/main"
	payload.Resource.CreatedBy = &AzureReposIdentity{
		ID:          "user-1",
		DisplayName: "Developer",
		UniqueName:  "dev@scandrix.dev",
	}

	act := adapter.MapAction(payload)
	if act != ActionOpened {
		t.Errorf("expected ActionOpened, got %s", act)
	}

	user := adapter.MapUser(payload)
	if user == nil || user.Login != "dev@scandrix.dev" {
		t.Fatalf("failed user mapping: %+v", user)
	}

	pr := adapter.MapPullRequest(payload)
	if pr == nil || pr.Number != 101 || pr.Title != "Fix memory leak" {
		t.Fatalf("failed pr mapping: %+v", pr)
	}
}
