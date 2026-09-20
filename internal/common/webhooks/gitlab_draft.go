package webhooks

// GitLabDraftLike contains flags indicating draft or work-in-progress status.
type GitLabDraftLike struct {
	Draft          *bool `json:"draft"`
	WorkInProgress *bool `json:"work_in_progress"`
}

// GitLabDraftChangeLike captures before-and-after values for boolean transitions.
type GitLabDraftChangeLike struct {
	Previous *bool `json:"previous"`
	Current  *bool `json:"current"`
}

// GitLabDraftChangesLike holds draft and work_in_progress transition diffs.
type GitLabDraftChangesLike struct {
	Draft          *GitLabDraftChangeLike `json:"draft"`
	WorkInProgress *GitLabDraftChangeLike `json:"work_in_progress"`
}

// ResolveGitLabDraftStatus returns true if either draft or work_in_progress is set to true.
func ResolveGitLabDraftStatus(mr *GitLabDraftLike) bool {
	if mr == nil {
		return false
	}
	if mr.Draft != nil {
		return *mr.Draft
	}
	if mr.WorkInProgress != nil {
		return *mr.WorkInProgress
	}
	return false
}

// IsGitLabDraftToReadyChange returns true if a merge request transitioned from Draft/WIP to Ready for review.
func IsGitLabDraftToReadyChange(changes *GitLabDraftChangesLike) bool {
	if changes == nil {
		return false
	}

	draftChange := changes.Draft
	if draftChange != nil && draftChange.Previous != nil && draftChange.Current != nil {
		if *draftChange.Previous && !*draftChange.Current {
			return true
		}
	}

	hasCompleteDraftChange := draftChange != nil && draftChange.Previous != nil && draftChange.Current != nil
	if hasCompleteDraftChange {
		return false
	}

	wipChange := changes.WorkInProgress
	if wipChange != nil && wipChange.Previous != nil && wipChange.Current != nil {
		return *wipChange.Previous && !*wipChange.Current
	}

	return false
}
