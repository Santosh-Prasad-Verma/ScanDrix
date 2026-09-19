package webhooks_test

import (
	"testing"

	"github.com/scandrix/backend/internal/common/webhooks"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestResolveGitLabDraftStatus_Comprehensive(t *testing.T) {
	t.Run("returns true when draft is true and work_in_progress is false", func(t *testing.T) {
		mr := &webhooks.GitLabDraftLike{
			Draft:          boolPtr(true),
			WorkInProgress: boolPtr(false),
		}
		if !webhooks.ResolveGitLabDraftStatus(mr) {
			t.Fatal("expected true")
		}
	})

	t.Run("returns false when draft is false even if work_in_progress is true", func(t *testing.T) {
		mr := &webhooks.GitLabDraftLike{
			Draft:          boolPtr(false),
			WorkInProgress: boolPtr(true),
		}
		if webhooks.ResolveGitLabDraftStatus(mr) {
			t.Fatal("expected false")
		}
	})

	t.Run("falls back to work_in_progress when draft is null", func(t *testing.T) {
		mr := &webhooks.GitLabDraftLike{
			Draft:          nil,
			WorkInProgress: boolPtr(true),
		}
		if !webhooks.ResolveGitLabDraftStatus(mr) {
			t.Fatal("expected true")
		}
	})

	t.Run("falls back to work_in_progress when draft is missing", func(t *testing.T) {
		mr := &webhooks.GitLabDraftLike{
			WorkInProgress: boolPtr(true),
		}
		if !webhooks.ResolveGitLabDraftStatus(mr) {
			t.Fatal("expected true")
		}
	})

	t.Run("returns false when both fields are missing", func(t *testing.T) {
		mr := &webhooks.GitLabDraftLike{}
		if webhooks.ResolveGitLabDraftStatus(mr) {
			t.Fatal("expected false")
		}
	})
}

func TestIsGitLabDraftToReadyChange_Comprehensive(t *testing.T) {
	t.Run("detects draft true to false", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			Draft: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  boolPtr(false),
			},
		}
		if !webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected true")
		}
	})

	t.Run("detects work_in_progress true to false", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			WorkInProgress: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  boolPtr(false),
			},
		}
		if !webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected true")
		}
	})

	t.Run("falls back to work_in_progress when draft change values are null", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			Draft: &webhooks.GitLabDraftChangeLike{
				Previous: nil,
				Current:  nil,
			},
			WorkInProgress: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  boolPtr(false),
			},
		}
		if !webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected true")
		}
	})

	t.Run("does not let work_in_progress override explicit draft change values", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			Draft: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(false),
				Current:  boolPtr(false),
			},
			WorkInProgress: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  boolPtr(false),
			},
		}
		if webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected false")
		}
	})

	t.Run("does not mix draft previous true with work_in_progress current false", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			Draft: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  nil,
			},
			WorkInProgress: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(false),
				Current:  boolPtr(false),
			},
		}
		if webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected false")
		}
	})

	t.Run("does not mix work_in_progress previous true with draft current false", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			Draft: &webhooks.GitLabDraftChangeLike{
				Previous: nil,
				Current:  boolPtr(false),
			},
			WorkInProgress: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  boolPtr(true),
			},
		}
		if webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected false")
		}
	})

	t.Run("falls back to work_in_progress when draft previous is null and current is false", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			Draft: &webhooks.GitLabDraftChangeLike{
				Previous: nil,
				Current:  boolPtr(false),
			},
			WorkInProgress: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  boolPtr(false),
			},
		}
		if !webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected true")
		}
	})

	t.Run("falls back to work_in_progress when draft previous is true and current is null", func(t *testing.T) {
		changes := &webhooks.GitLabDraftChangesLike{
			Draft: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  nil,
			},
			WorkInProgress: &webhooks.GitLabDraftChangeLike{
				Previous: boolPtr(true),
				Current:  boolPtr(false),
			},
		}
		if !webhooks.IsGitLabDraftToReadyChange(changes) {
			t.Fatal("expected true")
		}
	})
}
