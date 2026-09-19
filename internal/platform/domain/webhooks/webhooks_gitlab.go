// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

// GitLabUser models actors in GitLab webhook payloads.
type GitLabUser struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email,omitempty"`
}

// GitLabProject describes repository project fields in GitLab webhooks.
type GitLabProject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	WebURL            string `json:"web_url"`
	AvatarURL         string `json:"avatar_url,omitempty"`
	GitSSHURL         string `json:"git_ssh_url"`
	GitHTTPURL        string `json:"git_http_url"`
	Namespace         string `json:"namespace"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
	Homepage          string `json:"homepage,omitempty"`
	URL               string `json:"url"`
	SSHURL            string `json:"ssh_url"`
	HTTPURL           string `json:"http_url"`
}

// GitLabMergeRequest models the object_attributes payload for Merge Request hooks.
type GitLabMergeRequest struct {
	ID              int64              `json:"id"`
	IID             int                `json:"iid"`
	TargetBranch    string             `json:"target_branch"`
	SourceBranch    string             `json:"source_branch"`
	SourceProjectID int64              `json:"source_project_id"`
	AuthorID        int64              `json:"author_id"`
	AssigneeID      *int64             `json:"assignee_id,omitempty"`
	Title           string             `json:"title"`
	CreatedAt       string             `json:"created_at"`
	UpdatedAt       string             `json:"updated_at"`
	State           string             `json:"state"` // "opened", "closed", "merged", "locked"
	MergeStatus     string             `json:"merge_status"`
	TargetProjectID int64              `json:"target_project_id"`
	IIDString       string             `json:"iid_string,omitempty"`
	Description     string             `json:"description,omitempty"`
	Action          string             `json:"action,omitempty"` // "open", "update", "close", "reopen", "merge"
	WorkInProgress  bool               `json:"work_in_progress"`
	Draft           bool               `json:"draft"`
	URL             string             `json:"url"`
	OldRev          string             `json:"oldrev,omitempty"`
	LastCommit      *GitLabLastCommit  `json:"last_commit,omitempty"`
	Author          *GitLabUser        `json:"author,omitempty"`
}

// GitLabLastCommit carries commit info for MR head.
type GitLabLastCommit struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Title     string `json:"title"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
	Author    struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
}

// GitLabDraftChange indicates draft state transition.
type GitLabDraftChange struct {
	Previous bool `json:"previous"`
	Current  bool `json:"current"`
}

// GitLabChanges captures field mutations on merge requests.
type GitLabChanges struct {
	Draft       *GitLabDraftChange `json:"draft,omitempty"`
	Description any                `json:"description,omitempty"`
}

// GitLabMergeRequestPayload represents "Merge Request Hook" events.
type GitLabMergeRequestPayload struct {
	ObjectKind       string             `json:"object_kind"` // "merge_request"
	EventType        string             `json:"event_type"`
	User             GitLabUser         `json:"user"`
	Project          GitLabProject      `json:"project"`
	Repository       struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		Description string `json:"description,omitempty"`
		Homepage    string `json:"homepage,omitempty"`
	} `json:"repository"`
	ObjectAttributes GitLabMergeRequest `json:"object_attributes"`
	Changes          *GitLabChanges     `json:"changes,omitempty"`
	Labels           []struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"labels,omitempty"`
	Assignees []GitLabUser `json:"assignees,omitempty"`
	Reviewers []GitLabUser `json:"reviewers,omitempty"`
}

// GitLabNote models a comment or inline discussion note on an MR.
type GitLabNote struct {
	ID           int64   `json:"id"`
	Note         string  `json:"note"`
	NoteableType string  `json:"noteable_type"` // "MergeRequest", "Issue", "Commit"
	AuthorID     int64   `json:"author_id"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	ProjectID    int64   `json:"project_id"`
	Attachment   *string `json:"attachment,omitempty"`
	LineCode     *string `json:"line_code,omitempty"`
	CommitID     *string `json:"commit_id,omitempty"`
	NoteableID   int64   `json:"noteable_id"`
	System       bool    `json:"system"`
	StDiff       any     `json:"st_diff,omitempty"`
	URL          string  `json:"url"`
	Type         *string `json:"type,omitempty"` // "DiffNote", "DiscussionNote"
	Position     any     `json:"position,omitempty"`
}

// GitLabNotePayload represents "Note Hook" comment events.
type GitLabNotePayload struct {
	ObjectKind       string              `json:"object_kind"` // "note"
	EventType        string              `json:"event_type"`
	User             GitLabUser          `json:"user"`
	ProjectID        int64               `json:"project_id"`
	Project          GitLabProject       `json:"project"`
	ObjectAttributes GitLabNote          `json:"object_attributes"`
	MergeRequest     *GitLabMergeRequest `json:"merge_request,omitempty"`
}
