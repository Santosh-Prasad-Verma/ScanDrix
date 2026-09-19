// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

import "time"

// WebhookGithubPullRequestAction captures all GitHub PR event actions.
// Matches libs/platform/domain/platformIntegrations/types/webhooks/webhooks-github.type.ts
type WebhookGithubPullRequestAction string

const (
	GithubPRActionAssigned             WebhookGithubPullRequestAction = "assigned"
	GithubPRActionAutoMergeDisabled    WebhookGithubPullRequestAction = "auto_merge_disabled"
	GithubPRActionAutoMergeEnabled     WebhookGithubPullRequestAction = "auto_merge_enabled"
	GithubPRActionClosed               WebhookGithubPullRequestAction = "closed"
	GithubPRActionConvertedToDraft     WebhookGithubPullRequestAction = "converted_to_draft"
	GithubPRActionDemilestoned         WebhookGithubPullRequestAction = "demilestoned"
	GithubPRActionDequeued             WebhookGithubPullRequestAction = "dequeued"
	GithubPRActionEdited               WebhookGithubPullRequestAction = "edited"
	GithubPRActionEnqueued             WebhookGithubPullRequestAction = "enqueued"
	GithubPRActionLabeled              WebhookGithubPullRequestAction = "labeled"
	GithubPRActionLocked               WebhookGithubPullRequestAction = "locked"
	GithubPRActionMilestoned           WebhookGithubPullRequestAction = "milestoned"
	GithubPRActionOpened               WebhookGithubPullRequestAction = "opened"
	GithubPRActionReadyForReview       WebhookGithubPullRequestAction = "ready_for_review"
	GithubPRActionReopened             WebhookGithubPullRequestAction = "reopened"
	GithubPRActionReviewRequestRemoved WebhookGithubPullRequestAction = "review_request_removed"
	GithubPRActionReviewRequested      WebhookGithubPullRequestAction = "review_requested"
	GithubPRActionSynchronize          WebhookGithubPullRequestAction = "synchronize"
	GithubPRActionUnassigned           WebhookGithubPullRequestAction = "unassigned"
	GithubPRActionUnlabeled            WebhookGithubPullRequestAction = "unlabeled"
	GithubPRActionUnlocked             WebhookGithubPullRequestAction = "unlocked"
)

// WebhookGithubPullRequestCommentAction captures comment actions.
type WebhookGithubPullRequestCommentAction string

const (
	GithubCommentActionCreated WebhookGithubPullRequestCommentAction = "created"
	GithubCommentActionDeleted WebhookGithubPullRequestCommentAction = "deleted"
	GithubCommentActionEdited  WebhookGithubPullRequestCommentAction = "edited"
)

// GitHubUser models the author, sender, or assignee in GitHub webhooks.
type GitHubUser struct {
	Name         string `json:"name,omitempty"`
	Email        string `json:"email,omitempty"`
	ID           int64  `json:"id"`
	NodeID       string `json:"node_id"`
	Login        string `json:"login"`
	AvatarURL    string `json:"avatar_url"`
	GravatarID   string `json:"gravatar_id,omitempty"`
	URL          string `json:"url,omitempty"`
	HTMLURL      string `json:"html_url"`
	FollowersURL string `json:"followers_url,omitempty"`
	FollowingURL string `json:"following_url,omitempty"`
	GistsURL     string `json:"gists_url,omitempty"`
	StarredURL   string `json:"starred_url,omitempty"`
	Type         string `json:"type"` // "User", "Bot"
	SiteAdmin    bool   `json:"site_admin"`
	UserViewType string `json:"user_view_type,omitempty"`
}

// GitHubLabel describes issue/PR labels.
type GitHubLabel struct {
	ID          int64  `json:"id"`
	NodeID      string `json:"node_id"`
	URL         string `json:"url,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color"`
	Default     bool   `json:"default,omitempty"`
}

// GitHubMilestone describes milestones attached to issues or pull requests.
type GitHubMilestone struct {
	URL          string      `json:"url,omitempty"`
	HTMLURL      string      `json:"html_url,omitempty"`
	LabelsURL    string      `json:"labels_url,omitempty"`
	ID           int64       `json:"id"`
	NodeID       string      `json:"node_id"`
	Number       int         `json:"number"`
	State        string      `json:"state"` // "open" | "closed"
	Title        string      `json:"title"`
	Description  string      `json:"description,omitempty"`
	Creator      *GitHubUser `json:"creator,omitempty"`
	OpenIssues   int         `json:"open_issues,omitempty"`
	ClosedIssues int         `json:"closed_issues,omitempty"`
	CreatedAt    time.Time   `json:"created_at,omitempty"`
	UpdatedAt    time.Time   `json:"updated_at,omitempty"`
	ClosedAt     *time.Time  `json:"closed_at,omitempty"`
	DueOn        *time.Time  `json:"due_on,omitempty"`
}

// GitHubTeam describes a team in GitHub organization.
type GitHubTeam struct {
	ID                  int64  `json:"id"`
	NodeID              string `json:"node_id"`
	URL                 string `json:"url,omitempty"`
	MembersURL          string `json:"members_url,omitempty"`
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	Permission          string `json:"permission,omitempty"`
	Privacy             string `json:"privacy,omitempty"`
	NotificationSetting string `json:"notification_setting,omitempty"`
	HTMLURL             string `json:"html_url,omitempty"`
	RepositoriesURL     string `json:"repositories_url,omitempty"`
	Slug                string `json:"slug"`
	LdapDN              string `json:"ldap_dn,omitempty"`
}

// GitHubLicense captures repository license metadata.
type GitHubLicense struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	SPDXID  string `json:"spdx_id,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	HTMLURL string `json:"html_url,omitempty"`
}

// GitHubPermissions describes repository access permissions.
type GitHubPermissions struct {
	Admin    bool `json:"admin"`
	Pull     bool `json:"pull"`
	Triage   bool `json:"triage,omitempty"`
	Push     bool `json:"push"`
	Maintain bool `json:"maintain,omitempty"`
}

// GitHubAutoMerge details auto-merge settings if enabled on a PR.
type GitHubAutoMerge struct {
	EnabledBy     GitHubUser `json:"enabled_by"`
	MergeMethod   string     `json:"merge_method"` // "merge" | "squash" | "rebase"
	CommitTitle   string     `json:"commit_title,omitempty"`
	CommitMessage string     `json:"commit_message,omitempty"`
}

// GitHubBranchRef models head and base branches for a PR.
type GitHubBranchRef struct {
	Label string           `json:"label"`
	Ref   string           `json:"ref"`
	SHA   string           `json:"sha"`
	User  GitHubUser       `json:"user"`
	Repo  GitHubRepository `json:"repo"`
}

// GitHubRepository represents detailed repository fields in GitHub webhooks.
type GitHubRepository struct {
	ID                       int64              `json:"id"`
	NodeID                   string             `json:"node_id"`
	Name                     string             `json:"name"`
	FullName                 string             `json:"full_name"`
	Private                  bool               `json:"private"`
	Owner                    GitHubUser         `json:"owner"`
	HTMLURL                  string             `json:"html_url"`
	Description              string             `json:"description,omitempty"`
	Fork                     bool               `json:"fork"`
	URL                      string             `json:"url"`
	CloneURL                 string             `json:"clone_url"`
	MirrorURL                string             `json:"mirror_url,omitempty"`
	HooksURL                 string             `json:"hooks_url,omitempty"`
	SVNURL                   string             `json:"svn_url,omitempty"`
	Homepage                 string             `json:"homepage,omitempty"`
	Language                 string             `json:"language,omitempty"`
	ForksCount               int                `json:"forks_count,omitempty"`
	StargazersCount          int                `json:"stargazers_count,omitempty"`
	WatchersCount            int                `json:"watchers_count,omitempty"`
	Size                     int                `json:"size,omitempty"`
	DefaultBranch            string             `json:"default_branch"`
	OpenIssuesCount          int                `json:"open_issues_count,omitempty"`
	IsTemplate               bool               `json:"is_template,omitempty"`
	Topics                   []string           `json:"topics,omitempty"`
	HasIssues                bool               `json:"has_issues"`
	HasProjects              bool               `json:"has_projects"`
	HasWiki                  bool               `json:"has_wiki"`
	HasPages                 bool               `json:"has_pages,omitempty"`
	HasDownloads             bool               `json:"has_downloads,omitempty"`
	HasDiscussions           bool               `json:"has_discussions,omitempty"`
	Archived                 bool               `json:"archived"`
	Disabled                 bool               `json:"disabled"`
	Visibility               string             `json:"visibility,omitempty"`
	PushedAt                 *time.Time         `json:"pushed_at,omitempty"`
	CreatedAt                *time.Time         `json:"created_at,omitempty"`
	UpdatedAt                *time.Time         `json:"updated_at,omitempty"`
	AllowRebaseMerge         bool               `json:"allow_rebase_merge,omitempty"`
	AllowSquashMerge         bool               `json:"allow_squash_merge,omitempty"`
	AllowAutoMerge           bool               `json:"allow_auto_merge,omitempty"`
	DeleteBranchOnMerge      bool               `json:"delete_branch_on_merge,omitempty"`
	AllowUpdateBranch        bool               `json:"allow_update_branch,omitempty"`
	AllowMergeCommit         bool               `json:"allow_merge_commit,omitempty"`
	AllowForking             bool               `json:"allow_forking,omitempty"`
	WebCommitSignoffRequired bool               `json:"web_commit_signoff_required,omitempty"`
	License                  *GitHubLicense     `json:"license,omitempty"`
	Permissions              *GitHubPermissions `json:"permissions,omitempty"`
}

// GitHubPullRequest describes pull request payload fields.
type GitHubPullRequest struct {
	ID                  int64            `json:"ID"`
	NodeID              string           `json:"node_id"`
	Number              int              `json:"number"`
	State               string           `json:"state"` // "open", "closed"
	Locked              bool             `json:"locked"`
	Title               string           `json:"title"`
	User                GitHubUser       `json:"user"`
	Body                string           `json:"body"`
	Labels              []GitHubLabel    `json:"labels,omitempty"`
	Milestone           *GitHubMilestone `json:"milestone,omitempty"`
	ActiveLockReason    string           `json:"active_lock_reason,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
	ClosedAt            *time.Time       `json:"closed_at,omitempty"`
	MergedAt            *time.Time       `json:"merged_at,omitempty"`
	MergeCommitSHA      *string          `json:"merge_commit_sha,omitempty"`
	Assignee            *GitHubUser      `json:"assignee,omitempty"`
	Assignees           []GitHubUser     `json:"assignees,omitempty"`
	RequestedReviewers  []GitHubUser     `json:"requested_reviewers,omitempty"`
	RequestedTeams      []GitHubTeam     `json:"requested_teams,omitempty"`
	Head                GitHubBranchRef  `json:"head"`
	Base                GitHubBranchRef  `json:"base"`
	AuthorAssociation   string           `json:"author_association,omitempty"`
	AutoMerge           *GitHubAutoMerge `json:"auto_merge,omitempty"`
	Draft               bool             `json:"draft"`
	Merged              bool             `json:"merged"`
	Mergeable           *bool            `json:"mergeable,omitempty"`
	Rebasable           *bool            `json:"rebasable,omitempty"`
	MergeableState      string           `json:"mergeable_state,omitempty"`
	MergedBy            *GitHubUser      `json:"merged_by,omitempty"`
	Comments            int              `json:"comments"`
	ReviewComments      int              `json:"review_comments"`
	MaintainerCanModify bool             `json:"maintainer_can_modify,omitempty"`
	Commits             int              `json:"commits"`
	Additions           int              `json:"additions"`
	Deletions           int              `json:"deletions"`
	ChangedFiles        int              `json:"changed_files"`
	URL                 string           `json:"url,omitempty"`
	HTMLURL             string           `json:"html_url"`
	DiffURL             string           `json:"diff_url,omitempty"`
	PatchURL            string           `json:"patch_url,omitempty"`
	IssueURL            string           `json:"issue_url,omitempty"`
	CommitsURL          string           `json:"commits_url,omitempty"`
	ReviewCommentsURL   string           `json:"review_comments_url,omitempty"`
	ReviewCommentURL    string           `json:"review_comment_url,omitempty"`
	CommentsURL         string           `json:"comments_url,omitempty"`
	StatusesURL         string           `json:"statuses_url,omitempty"`
}

// GitHubCommentReactions captures reaction counts on PR or issue comments.
type GitHubCommentReactions struct {
	PlusOne    int    `json:"+1"`
	MinusOne   int    `json:"-1"`
	Confused   int    `json:"confused"`
	Eyes       int    `json:"eyes"`
	Heart      int    `json:"heart"`
	Hooray     int    `json:"hooray"`
	Laugh      int    `json:"laugh"`
	Rocket     int    `json:"rocket"`
	TotalCount int    `json:"total_count"`
	URL        string `json:"url,omitempty"`
}

// GitHubComment represents comments made in PR conversations or inline code discussions.
type GitHubComment struct {
	ID                  int64                   `json:"id"`
	NodeID              string                  `json:"node_id"`
	URL                 string                  `json:"url"`
	HTMLURL             string                  `json:"html_url"`
	Body                string                  `json:"body"`
	User                GitHubUser              `json:"user"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
	DiffHunk            string                  `json:"diff_hunk,omitempty"`
	Path                string                  `json:"path,omitempty"`
	Position            *int                    `json:"position,omitempty"`
	OriginalPosition    *int                    `json:"original_position,omitempty"`
	CommitID            string                  `json:"commit_id,omitempty"`
	OriginalCommitID    string                  `json:"original_commit_id,omitempty"`
	InReplyToID         *int64                  `json:"in_reply_to_id,omitempty"`
	PullRequestReviewID *int64                  `json:"pull_request_review_id,omitempty"`
	PullRequestURL      string                  `json:"pull_request_url,omitempty"`
	StartLine           *int                    `json:"start_line,omitempty"`
	Line                *int                    `json:"line,omitempty"`
	OriginalLine        *int                    `json:"original_line,omitempty"`
	OriginalStartLine   *int                    `json:"original_start_line,omitempty"`
	Side                string                  `json:"side,omitempty"`       // "LEFT" | "RIGHT"
	StartSide           string                  `json:"start_side,omitempty"` // "LEFT" | "RIGHT"
	SubjectType         string                  `json:"subject_type,omitempty"`
	AuthorAssociation   string                  `json:"author_association,omitempty"`
	Reactions           *GitHubCommentReactions `json:"reactions,omitempty"`
}

// GitHubIssue captures issue fields when comments are posted on PR issue threads.
type GitHubIssue struct {
	ID               int64                   `json:"id"`
	NodeID           string                  `json:"node_id"`
	Number           int                     `json:"number"`
	Title            string                  `json:"title"`
	User             GitHubUser              `json:"user"`
	Labels           []GitHubLabel           `json:"labels,omitempty"`
	State            string                  `json:"state"`
	Locked           bool                    `json:"locked"`
	Comments         int                     `json:"comments"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
	ClosedAt         *time.Time              `json:"closed_at,omitempty"`
	Body             string                  `json:"body"`
	ActiveLockReason string                  `json:"active_lock_reason,omitempty"`
	Milestone        *GitHubMilestone        `json:"milestone,omitempty"`
	PullRequest      *GitHubPullRequestStub  `json:"pull_request,omitempty"`
	Reactions        *GitHubCommentReactions `json:"reactions,omitempty"`
	RepositoryURL    string                  `json:"repository_url,omitempty"`
}

// GitHubPullRequestStub confirms that an issue is in fact a pull request.
type GitHubPullRequestStub struct {
	URL      string `json:"url"`
	HTMLURL  string `json:"html_url"`
	DiffURL  string `json:"diff_url,omitempty"`
	PatchURL string `json:"patch_url,omitempty"`
}

// GitHubInstallationInfo carries GitHub App installation id.
type GitHubInstallationInfo struct {
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
}

// GitHubPullRequestPayload represents pull_request event webhook payloads.
type GitHubPullRequestPayload struct {
	Action       string                  `json:"action"` // "opened", "synchronize", "closed", "reopened", "edited", "ready_for_review"
	Number       int                     `json:"number"`
	Before       string                  `json:"before,omitempty"`
	After        string                  `json:"after,omitempty"`
	PullRequest  GitHubPullRequest       `json:"pull_request"`
	Repository   GitHubRepository        `json:"repository"`
	Sender       GitHubUser              `json:"sender"`
	Installation *GitHubInstallationInfo `json:"installation,omitempty"`
	Organization any                     `json:"organization,omitempty"`
}

// GitHubIssueCommentPayload represents issue_comment webhook payloads.
type GitHubIssueCommentPayload struct {
	Action       string                  `json:"action"` // "created", "edited", "deleted"
	Issue        GitHubIssue             `json:"issue"`
	Comment      GitHubComment           `json:"comment"`
	Repository   GitHubRepository        `json:"repository"`
	Sender       GitHubUser              `json:"sender"`
	Installation *GitHubInstallationInfo `json:"installation,omitempty"`
	Organization any                     `json:"organization,omitempty"`
}

// GitHubReviewCommentPayload represents pull_request_review_comment payloads.
type GitHubReviewCommentPayload struct {
	Action       string                  `json:"action"` // "created", "edited", "deleted"
	PullRequest  GitHubPullRequest       `json:"pull_request"`
	Comment      GitHubComment           `json:"comment"`
	Repository   GitHubRepository        `json:"repository"`
	Sender       GitHubUser              `json:"sender"`
	Installation *GitHubInstallationInfo `json:"installation,omitempty"`
	Organization any                     `json:"organization,omitempty"`
}

// GitHubPullRequestReview captures review payload on PR.
type GitHubPullRequestReview struct {
	ID                int64      `json:"id"`
	NodeID            string     `json:"node_id"`
	User              GitHubUser `json:"user"`
	Body              string     `json:"body"`
	CommitID          string     `json:"commit_id"`
	SubmittedAt       time.Time  `json:"submitted_at"`
	State             string     `json:"state"` // "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED"
	HTMLURL           string     `json:"html_url"`
	PullRequestURL    string     `json:"pull_request_url"`
	AuthorAssociation string     `json:"author_association"`
}

// GitHubPullRequestReviewEvent represents pull_request_review event payloads.
type GitHubPullRequestReviewEvent struct {
	Action       string                  `json:"action"` // "submitted", "edited", "dismissed"
	Review       GitHubPullRequestReview `json:"review"`
	PullRequest  GitHubPullRequest       `json:"pull_request"`
	Repository   GitHubRepository        `json:"repository"`
	Sender       GitHubUser              `json:"sender"`
	Installation *GitHubInstallationInfo `json:"installation,omitempty"`
	Organization any                     `json:"organization,omitempty"`
}

// GitHubCheckRunPayload represents check_run webhook events.
type GitHubCheckRunPayload struct {
	Action       string                  `json:"action"` // "created", "completed", "rerequested", "requested_action"
	CheckRun     GitHubCheckRun          `json:"check_run"`
	Repository   GitHubRepository        `json:"repository"`
	Sender       GitHubUser              `json:"sender"`
	Installation *GitHubInstallationInfo `json:"installation,omitempty"`
}

// GitHubCheckRun models a GitHub Check Run.
type GitHubCheckRun struct {
	ID          int64             `json:"id"`
	HeadSHA     string            `json:"head_sha"`
	NodeID      string            `json:"node_id"`
	ExternalID  string            `json:"external_id"`
	URL         string            `json:"url"`
	HTMLURL     string            `json:"html_url"`
	Status      string            `json:"status"`     // "queued", "in_progress", "completed"
	Conclusion  *string           `json:"conclusion"` // "success", "failure", "neutral", "cancelled", "timed_out", "action_required"
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Output      GitHubCheckOutput `json:"output"`
	Name        string            `json:"name"`
	CheckSuite  struct {
		ID int64 `json:"id"`
	} `json:"check_suite"`
}

// GitHubCheckOutput describes check output title, summary, and annotations.
type GitHubCheckOutput struct {
	Title            string                  `json:"title"`
	Summary          string                  `json:"summary"`
	Text             string                  `json:"text,omitempty"`
	AnnotationsCount int                     `json:"annotations_count"`
	AnnotationsURL   string                  `json:"annotations_url"`
	Annotations      []GitHubCheckAnnotation `json:"annotations,omitempty"`
}

// GitHubCheckAnnotation locates inline review warnings or failures.
type GitHubCheckAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	StartColumn     *int   `json:"start_column,omitempty"`
	EndColumn       *int   `json:"end_column,omitempty"`
	AnnotationLevel string `json:"annotation_level"` // "notice", "warning", "failure"
	Message         string `json:"message"`
	Title           string `json:"title,omitempty"`
	RawDetails      string `json:"raw_details,omitempty"`
}
