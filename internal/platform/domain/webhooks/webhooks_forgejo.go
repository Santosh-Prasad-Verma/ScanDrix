// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

// WebhookForgejoMilestoneState defines milestone state in Forgejo/Gitea.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/issue.go#L21
type WebhookForgejoMilestoneState string

const (
	ForgejoMilestoneOpen   WebhookForgejoMilestoneState = "open"
	ForgejoMilestoneClosed WebhookForgejoMilestoneState = "closed"
	ForgejoMilestoneAll    WebhookForgejoMilestoneState = "all"
)

// WebhookForgejoReviewState defines review decision in Forgejo/Gitea.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/pull_review.go#L13
type WebhookForgejoReviewState string

const (
	ForgejoReviewApproved       WebhookForgejoReviewState = "APPROVED"
	ForgejoReviewPending        WebhookForgejoReviewState = "PENDING"
	ForgejoReviewComment        WebhookForgejoReviewState = "COMMENT"
	ForgejoReviewRequestChanges WebhookForgejoReviewState = "REQUEST_CHANGES"
	ForgejoReviewRequestReview  WebhookForgejoReviewState = "REQUEST_REVIEW"
	ForgejoReviewUnknown        WebhookForgejoReviewState = ""
)

// WebhookForgejoHookIssueAction defines the trigger action for issues/PRs.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L291
type WebhookForgejoHookIssueAction string

const (
	ForgejoActionOpened               WebhookForgejoHookIssueAction = "opened"
	ForgejoActionClosed               WebhookForgejoHookIssueAction = "closed"
	ForgejoActionReopened             WebhookForgejoHookIssueAction = "reopened"
	ForgejoActionEdited               WebhookForgejoHookIssueAction = "edited"
	ForgejoActionAssigned             WebhookForgejoHookIssueAction = "assigned"
	ForgejoActionUnassigned           WebhookForgejoHookIssueAction = "unassigned"
	ForgejoActionLabelUpdated         WebhookForgejoHookIssueAction = "label_updated"
	ForgejoActionLabelCleared         WebhookForgejoHookIssueAction = "label_cleared"
	ForgejoActionSynchronized         WebhookForgejoHookIssueAction = "synchronized" // forgejo uses synchronized
	ForgejoActionMilestoned           WebhookForgejoHookIssueAction = "milestoned"
	ForgejoActionDemilestoned         WebhookForgejoHookIssueAction = "demilestoned"
	ForgejoActionReviewed             WebhookForgejoHookIssueAction = "reviewed"
	ForgejoActionReviewRequested      WebhookForgejoHookIssueAction = "review_requested"
	ForgejoActionReviewRequestRemoved WebhookForgejoHookIssueAction = "review_request_removed"
)

// WebhookForgejoCommentAction captures comment actions in Forgejo.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L197
type WebhookForgejoCommentAction string

const (
	ForgejoCommentActionCreated WebhookForgejoCommentAction = "created"
	ForgejoCommentActionEdited  WebhookForgejoCommentAction = "edited"
	ForgejoCommentActionDeleted WebhookForgejoCommentAction = "deleted"
)

// WebhookForgejoEvent lists standard event header types.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/actions/github.go#L10
type WebhookForgejoEvent string

const (
	ForgejoEventPullRequest             WebhookForgejoEvent = "pull_request"
	ForgejoEventPullRequestTarget       WebhookForgejoEvent = "pull_request_target"
	ForgejoEventPullRequestReviewComment WebhookForgejoEvent = "pull_request_review_comment"
	ForgejoEventPullRequestReview       WebhookForgejoEvent = "pull_request_review"
	ForgejoEventRegistryPackage         WebhookForgejoEvent = "registry_package"
	ForgejoEventCreate                  WebhookForgejoEvent = "create"
	ForgejoEventDelete                  WebhookForgejoEvent = "delete"
	ForgejoEventFork                    WebhookForgejoEvent = "fork"
	ForgejoEventPush                    WebhookForgejoEvent = "push"
	ForgejoEventIssues                  WebhookForgejoEvent = "issues"
	ForgejoEventIssueComment            WebhookForgejoEvent = "issue_comment"
	ForgejoEventRelease                 WebhookForgejoEvent = "release"
	ForgejoEventPullRequestComment      WebhookForgejoEvent = "pull_request_comment"
	ForgejoEventGollum                  WebhookForgejoEvent = "gollum"
	ForgejoEventSchedule                WebhookForgejoEvent = "schedule"
	ForgejoEventWorkflowDispatch        WebhookForgejoEvent = "workflow_dispatch"
)

// WebhookForgejoUser represents a Forgejo user account.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/user.go#L14
type WebhookForgejoUser struct {
	ID                int    `json:"id"`
	SourceID          int    `json:"source_id"`
	Login             string `json:"login"`
	LoginName         string `json:"login_name"`
	FullName          string `json:"full_name"`
	Email             string `json:"email"`
	AvatarURL         string `json:"avatar_url"`
	HTMLURL           string `json:"html_url"`
	Language          string `json:"language,omitempty"`
	Location          string `json:"location,omitempty"`
	Pronouns          string `json:"pronouns,omitempty"`
	Website           string `json:"website,omitempty"`
	Description       string `json:"description,omitempty"`
	Visibility        string `json:"visibility"` // "public" | "limited" | "private"
	IsAdmin           bool   `json:"is_admin"`
	Restricted        bool   `json:"restricted"`
	Active            bool   `json:"active"`
	ProhibitLogin     bool   `json:"prohibit_login"`
	LastLogin         string `json:"last_login,omitempty"`
	Created           string `json:"created,omitempty"`
	FollowersCount    int    `json:"followers_count"`
	FollowingCount    int    `json:"following_count"`
	StarredReposCount int    `json:"starred_repos_count"`
}

// WebhookForgejoLabel represents issue or PR label in Forgejo.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/issue_label.go#L13
type WebhookForgejoLabel struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Exclusive   bool   `json:"exclusive,omitempty"`
	IsArchived  bool   `json:"is_archived,omitempty"`
}

// WebhookForgejoMilestone represents milestone tracking.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/issue_milestone.go#L11
type WebhookForgejoMilestone struct {
	ID           int                          `json:"id"`
	Title        string                       `json:"title"`
	Description  string                       `json:"description"`
	State        WebhookForgejoMilestoneState `json:"state"`
	OpenIssues   int                          `json:"open_issues"`
	ClosedIssues int                          `json:"closed_issues"`
	CreatedAt    string                       `json:"created_at"`
	UpdatedAt    string                       `json:"updated_at,omitempty"`
	ClosedAt     string                       `json:"closed_at,omitempty"`
	DueOn        string                       `json:"due_on,omitempty"`
}

// WebhookForgejoPermission describes repository permissions.
type WebhookForgejoPermission struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

// WebhookForgejoInternalTracker describes internal issue tracker settings.
type WebhookForgejoInternalTracker struct {
	EnableTimeTracker                  bool `json:"enable_time_tracker"`
	AllowOnlyContributorsToTrackTime   bool `json:"allow_only_contributors_to_track_time"`
	EnableIssueDependencies            bool `json:"enable_issue_dependencies"`
}

// WebhookForgejoExternalTracker describes external issue tracker settings.
type WebhookForgejoExternalTracker struct {
	ExternalTrackerURL           string `json:"external_tracker_url"`
	ExternalTrackerFormat        string `json:"external_tracker_format"`
	ExternalTrackerStyle         string `json:"external_tracker_style"`
	ExternalTrackerRegexpPattern string `json:"external_tracker_regexp_pattern,omitempty"`
}

// WebhookForgejoExternalWiki describes external wiki settings.
type WebhookForgejoExternalWiki struct {
	ExternalWikiURL string `json:"external_wiki_url"`
}

// WebhookForgejoOrganization describes an organization in Forgejo.
type WebhookForgejoOrganization struct {
	ID                         int    `json:"id"`
	Name                       string `json:"name"`
	FullName                   string `json:"full_name"`
	Email                      string `json:"email"`
	AvatarURL                  string `json:"avatar_url"`
	Description                string `json:"description"`
	Website                    string `json:"website"`
	Location                   string `json:"location"`
	Visibility                 string `json:"visibility"`
	RepoAdminChangeTeamAccess  bool   `json:"repo_admin_change_team_access"`
}

// WebhookForgejoTeam describes an organization team.
type WebhookForgejoTeam struct {
	ID                      int                         `json:"id"`
	Name                    string                      `json:"name"`
	Description             string                      `json:"description"`
	Organization            *WebhookForgejoOrganization `json:"organization,omitempty"`
	IncludesAllRepositories bool                        `json:"includes_all_repositories"`
	Permission              string                      `json:"permission"` // "none" | "read" | "write" | "admin" | "owner"
	Units                   []string                    `json:"units,omitempty"`
	UnitsMap                map[string]string           `json:"units_map,omitempty"`
	CanCreateOrgRepo        bool                        `json:"can_create_org_repo"`
}

// WebhookForgejoRepoTransfer describes repository transfer information.
type WebhookForgejoRepoTransfer struct {
	Doer      *WebhookForgejoUser  `json:"doer,omitempty"`
	Recipient *WebhookForgejoUser  `json:"recipient,omitempty"`
	Teams     []WebhookForgejoTeam `json:"teams,omitempty"`
}

// WebhookForgejoRepository represents repository metadata in Forgejo.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/repo.go#L50
type WebhookForgejoRepository struct {
	ID                         int                            `json:"id"`
	Owner                      WebhookForgejoUser             `json:"owner"`
	Name                       string                         `json:"name"`
	FullName                   string                         `json:"full_name"`
	Description                string                         `json:"description"`
	Empty                      bool                           `json:"empty"`
	Private                    bool                           `json:"private"`
	Fork                       bool                           `json:"fork"`
	Template                   bool                           `json:"template"`
	Parent                     *WebhookForgejoRepository      `json:"parent,omitempty"`
	Mirror                     bool                           `json:"mirror,omitempty"`
	Size                       int                            `json:"size"`
	Language                   string                         `json:"language"`
	LanguagesURL               string                         `json:"languages_url,omitempty"`
	HTMLURL                    string                         `json:"html_url"`
	URL                        string                         `json:"url"`
	Link                       string                         `json:"link,omitempty"`
	SSHURL                     string                         `json:"ssh_url"`
	CloneURL                   string                         `json:"clone_url"`
	OriginalURL                string                         `json:"original_url,omitempty"`
	Website                    string                         `json:"website,omitempty"`
	StarsCount                 int                            `json:"stars_count"`
	ForksCount                 int                            `json:"forks_count"`
	WatchersCount              int                            `json:"watchers_count"`
	OpenIssuesCount            int                            `json:"open_issues_count"`
	OpenPRCounter              int                            `json:"open_pr_counter"`
	ReleaseCounter             int                            `json:"release_counter"`
	DefaultBranch              string                         `json:"default_branch"`
	Archived                   bool                           `json:"archived"`
	CreatedAt                  string                         `json:"created_at"`
	UpdatedAt                  string                         `json:"updated_at"`
	ArchivedAt                 string                         `json:"archived_at,omitempty"`
	Permissions                *WebhookForgejoPermission      `json:"permissions,omitempty"`
	HasIssues                  bool                           `json:"has_issues"`
	InternalTracker            *WebhookForgejoInternalTracker `json:"internal_tracker,omitempty"`
	ExternalTracker            *WebhookForgejoExternalTracker `json:"external_tracker,omitempty"`
	HasWiki                    bool                           `json:"has_wiki"`
	ExternalWiki               *WebhookForgejoExternalWiki    `json:"external_wiki,omitempty"`
	WikiBranch                 string                         `json:"wiki_branch,omitempty"`
	GloballyEditableWiki       bool                           `json:"globally_editable_wiki,omitempty"`
	HasPullRequests            bool                           `json:"has_pull_requests"`
	HasProjects                bool                           `json:"has_projects,omitempty"`
	HasReleases                bool                           `json:"has_releases,omitempty"`
	HasPackages                bool                           `json:"has_packages,omitempty"`
	HasActions                 bool                           `json:"has_actions,omitempty"`
	IgnoreWhitespaceConflicts  bool                           `json:"ignore_whitespace_conflicts,omitempty"`
	AllowMergeCommits          bool                           `json:"allow_merge_commits,omitempty"`
	AllowRebase                bool                           `json:"allow_rebase,omitempty"`
	AllowRebaseExplicit        bool                           `json:"allow_rebase_explicit,omitempty"`
	AllowSquashMerge           bool                           `json:"allow_squash_merge,omitempty"`
	AllowFastForwardOnlyMerge  bool                           `json:"allow_fast_forward_only_merge,omitempty"`
	AllowRebaseUpdate          bool                           `json:"allow_rebase_update,omitempty"`
	DefaultDeleteBranchAfterMerge bool                        `json:"default_delete_branch_after_merge,omitempty"`
	DefaultMergeStyle          string                         `json:"default_merge_style,omitempty"`
	DefaultAllowMaintainerEdit bool                           `json:"default_allow_maintainer_edit,omitempty"`
	DefaultUpdateStyle         string                         `json:"default_update_style,omitempty"`
	AvatarURL                  string                         `json:"avatar_url"`
	Internal                   bool                           `json:"internal"`
	MirrorInterval             string                         `json:"mirror_interval,omitempty"`
	ObjectFormatName           string                         `json:"object_format_name,omitempty"`
	MirrorUpdated              string                         `json:"mirror_updated,omitempty"`
	RepoTransfer               *WebhookForgejoRepoTransfer    `json:"repo_transfer,omitempty"`
	Topics                     []string                       `json:"topics,omitempty"`
}

// WebhookForgejoBranchInfo holds branch ref and commit hash.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/pull.go#L65
type WebhookForgejoBranchInfo struct {
	Label  string                   `json:"label"`
	Ref    string                   `json:"ref"`
	SHA    string                   `json:"sha"`
	RepoID int                      `json:"repo_id"`
	Repo   WebhookForgejoRepository `json:"repo"`
}

// WebhookForgejoAttachment represents an uploaded asset or attachment.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/attachment.go#L12
type WebhookForgejoAttachment struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Size               int    `json:"size"`
	DownloadCount      int    `json:"download_count"`
	CreatedAt          string `json:"created_at"`
	UUID               string `json:"uuid"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Type               string `json:"type"` // "attachment" | "external"
}

// WebhookForgejoPullRequest represents a pull request in Forgejo/Gitea.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/pull.go#L11
type WebhookForgejoPullRequest struct {
	ID                     int                          `json:"id"`
	URL                    string                       `json:"url"`
	Number                 int                          `json:"number"`
	User                   WebhookForgejoUser           `json:"user"`
	Title                  string                       `json:"title"`
	Body                   string                       `json:"body"`
	Labels                 []WebhookForgejoLabel        `json:"labels,omitempty"`
	Milestone              *WebhookForgejoMilestone     `json:"milestone,omitempty"`
	Assignee               *WebhookForgejoUser          `json:"assignee,omitempty"`
	Assignees              []WebhookForgejoUser         `json:"assignees,omitempty"`
	RequestedReviewers     []WebhookForgejoUser         `json:"requested_reviewers,omitempty"`
	RequestedReviewerTeams []WebhookForgejoTeam         `json:"requested_reviewers_teams,omitempty"`
	State                  WebhookForgejoMilestoneState `json:"state"`
	Draft                  bool                         `json:"draft"`
	IsLocked               bool                         `json:"is_locked"`
	Comments               int                          `json:"comments"`
	ReviewComments         int                          `json:"review_comments"`
	Additions              int                          `json:"additions"`
	Deletions              int                          `json:"deletions"`
	ChangedFiles           int                          `json:"changed_files"`
	HTMLURL                string                       `json:"html_url"`
	DiffURL                string                       `json:"diff_url"`
	PatchURL               string                       `json:"patch_url"`
	Mergeable              bool                         `json:"mergeable"`
	Merged                 bool                         `json:"merged"`
	MergedAt               *string                      `json:"merged_at,omitempty"`
	MergeCommitSHA         *string                      `json:"merge_commit_sha,omitempty"`
	MergedBy               *WebhookForgejoUser          `json:"merged_by,omitempty"`
	AllowMaintainerEdit    bool                         `json:"allow_maintainer_edit"`
	Base                   WebhookForgejoBranchInfo     `json:"base"`
	Head                   WebhookForgejoBranchInfo     `json:"head"`
	MergeBase              string                       `json:"merge_base"`
	DueDate                *string                      `json:"due_date,omitempty"`
	CreatedAt              string                       `json:"created_at"`
	UpdatedAt              string                       `json:"updated_at"`
	ClosedAt               *string                      `json:"closed_at,omitempty"`
	PinOrder               int                          `json:"pin_order,omitempty"`
	Flow                   int                          `json:"flow,omitempty"`
}

// WebhookForgejoComment represents a comment inside an issue or PR.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/issue_comment.go#L11
type WebhookForgejoComment struct {
	ID               int                        `json:"id"`
	HTMLURL          string                     `json:"html_url"`
	PullRequestURL   string                     `json:"pull_request_url,omitempty"`
	IssueURL         string                     `json:"issue_url,omitempty"`
	User             WebhookForgejoUser         `json:"user"`
	OriginalAuthor   string                     `json:"original_author,omitempty"`
	OriginalAuthorID int                        `json:"original_author_id,omitempty"`
	Body             string                     `json:"body"`
	Assets           []WebhookForgejoAttachment `json:"assets,omitempty"`
	CreatedAt        string                     `json:"created_at"`
	UpdatedAt        string                     `json:"updated_at"`
}

// WebhookForgejoReview represents a pull request review.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/pull_review.go#L29
type WebhookForgejoReview struct {
	ID             int                       `json:"id"`
	User           WebhookForgejoUser        `json:"user"`
	Team           *WebhookForgejoTeam       `json:"team,omitempty"`
	State          WebhookForgejoReviewState `json:"state"`
	Body           string                    `json:"body"`
	CommitID       string                    `json:"commit_id"`
	Stale          bool                      `json:"stale"`
	Official       bool                      `json:"official"`
	Dismissed      bool                      `json:"dismissed"`
	CommentsCount  int                       `json:"comments_count"`
	SubmittedAt    string                    `json:"submitted_at"`
	UpdatedAt      string                    `json:"updated_at"`
	HTMLURL        string                    `json:"html_url"`
	PullRequestURL string                    `json:"pull_request_url"`
}

// WebhookForgejoCommitUser captures author/committer info.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L74
type WebhookForgejoCommitUser struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// WebhookForgejoPayloadCommitVerification verifies commit signatures.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L102
type WebhookForgejoPayloadCommitVerification struct {
	Verified  bool                      `json:"verified"`
	Reason    string                    `json:"reason"`
	Signature string                    `json:"signature"`
	Signer    *WebhookForgejoCommitUser `json:"signer,omitempty"`
	Payload   string                    `json:"payload"`
}

// WebhookForgejoCommit describes a Git commit in Forgejo hooks.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L86
type WebhookForgejoCommit struct {
	ID           string                                   `json:"id"`
	Message      string                                   `json:"message"`
	URL          string                                   `json:"url"`
	Author       WebhookForgejoCommitUser                 `json:"author"`
	Committer    WebhookForgejoCommitUser                 `json:"committer"`
	Verification *WebhookForgejoPayloadCommitVerification `json:"verification,omitempty"`
	Timestamp    string                                   `json:"timestamp"`
	Added        []string                                 `json:"added"`
	Removed      []string                                 `json:"removed"`
	Modified     []string                                 `json:"modified"`
}

// WebhookForgejoPushEvent represents push webhook events.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L311
type WebhookForgejoPushEvent struct {
	Ref          string                   `json:"ref"`
	Before       string                   `json:"before"`
	After        string                   `json:"after"`
	CompareURL   string                   `json:"compare_url"`
	Commits      []WebhookForgejoCommit   `json:"commits"`
	TotalCommits int                      `json:"total_commits"`
	HeadCommit   *WebhookForgejoCommit    `json:"head_commit,omitempty"`
	Repository   WebhookForgejoRepository `json:"repository"`
	Pusher       WebhookForgejoCommitUser `json:"pusher"`
	Sender       WebhookForgejoUser       `json:"sender"`
}

// WebhookForgejoChangesFromPayload describes previous value before change.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L340
type WebhookForgejoChangesFromPayload struct {
	From string `json:"from"`
}

// WebhookForgejoChangesPayload holds modified fields.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L345
type WebhookForgejoChangesPayload struct {
	Title *WebhookForgejoChangesFromPayload `json:"title,omitempty"`
	Body  *WebhookForgejoChangesFromPayload `json:"body,omitempty"`
	Ref   *WebhookForgejoChangesFromPayload `json:"ref,omitempty"`
}

// WebhookForgejoReviewPayload captures inline review metadata in review events.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L396
type WebhookForgejoReviewPayload struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// WebhookForgejoPullRequestEvent represents the webhook payload for PR events.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L359
type WebhookForgejoPullRequestEvent struct {
	Action      WebhookForgejoHookIssueAction `json:"action"`
	Number      int                           `json:"number"`
	Changes     *WebhookForgejoChangesPayload `json:"changes,omitempty"`
	PullRequest WebhookForgejoPullRequest     `json:"pull_request"`
	Repository  WebhookForgejoRepository      `json:"repository"`
	Sender      WebhookForgejoUser            `json:"sender"`
	CommitID    string                        `json:"commit_id,omitempty"`
	Review      *WebhookForgejoReview         `json:"review,omitempty"`
	Label       *WebhookForgejoLabel          `json:"label,omitempty"`
}

// WebhookForgejoPullRequestReviewEvent represents PR review webhooks.
type WebhookForgejoPullRequestReviewEvent struct {
	Action            WebhookForgejoHookIssueAction `json:"action"`
	Number            int                           `json:"number"`
	Changes           *WebhookForgejoChangesPayload `json:"changes,omitempty"`
	PullRequest       WebhookForgejoPullRequest     `json:"pull_request"`
	RequestedReviewer *WebhookForgejoUser           `json:"requested_reviewer,omitempty"`
	Repository        WebhookForgejoRepository      `json:"repository"`
	Sender            WebhookForgejoUser            `json:"sender"`
	CommitID          string                        `json:"commit_id,omitempty"`
	Review            WebhookForgejoReviewPayload   `json:"review"`
	Label             *WebhookForgejoLabel          `json:"label,omitempty"`
}

// WebhookForgejoPullRequestMeta represents PR reference inside an issue.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/issue.go#L29
type WebhookForgejoPullRequestMeta struct {
	Merged   bool   `json:"merged"`
	MergedAt string `json:"merged_at,omitempty"`
	Draft    bool   `json:"draft"`
	HTMLURL  string `json:"html_url"`
}

// WebhookForgejoRepositoryMeta represents repository reference inside an issue.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/issue.go#L37
type WebhookForgejoRepositoryMeta struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	FullName string `json:"full_name"`
}

// WebhookForgejoIssue represents an issue in Forgejo.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/issue.go#L46
type WebhookForgejoIssue struct {
	ID               int                            `json:"id"`
	URL              string                         `json:"url"`
	HTMLURL          string                         `json:"html_url"`
	Number           int                            `json:"number"`
	User             WebhookForgejoUser             `json:"user"`
	OriginalAuthor   string                         `json:"original_author,omitempty"`
	OriginalAuthorID int                            `json:"original_author_id,omitempty"`
	Title            string                         `json:"title"`
	Body             string                         `json:"body"`
	Ref              string                         `json:"ref,omitempty"`
	Assets           []WebhookForgejoAttachment     `json:"assets,omitempty"`
	Labels           []WebhookForgejoLabel          `json:"labels,omitempty"`
	Milestone        *WebhookForgejoMilestone       `json:"milestone,omitempty"`
	Assignee         *WebhookForgejoUser            `json:"assignee,omitempty"`
	Assignees        []WebhookForgejoUser           `json:"assignees,omitempty"`
	State            WebhookForgejoMilestoneState   `json:"state"`
	IsLocked         bool                           `json:"is_locked"`
	Comments         int                            `json:"comments"`
	CreatedAt        string                         `json:"created_at"`
	UpdatedAt        string                         `json:"updated_at"`
	ClosedAt         *string                        `json:"closed_at,omitempty"`
	DueDate          *string                        `json:"due_date,omitempty"`
	PullRequest      *WebhookForgejoPullRequestMeta `json:"pull_request,omitempty"`
	Repository       *WebhookForgejoRepositoryMeta  `json:"repository,omitempty"`
	PinOrder         int                            `json:"pin_order,omitempty"`
}

// WebhookForgejoIssueCommentEvent represents the webhook payload for issue/PR comments.
// @see https://codeberg.org/forgejo/forgejo/src/branch/forgejo/modules/structs/hook.go#L204
type WebhookForgejoIssueCommentEvent struct {
	Action      WebhookForgejoCommentAction   `json:"action"` // "created", "edited", "deleted"
	Issue       WebhookForgejoIssue           `json:"issue"`
	PullRequest *WebhookForgejoPullRequest    `json:"pull_request,omitempty"`
	Comment     WebhookForgejoComment         `json:"comment"`
	Changes     *WebhookForgejoChangesPayload `json:"changes,omitempty"`
	Repository  WebhookForgejoRepository      `json:"repository"`
	Sender      WebhookForgejoUser            `json:"sender"`
	IsPull      bool                          `json:"is_pull"`
}
