// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

import "strings"

// WebhookBitbucketPullRequestState represents state of a Bitbucket PR.
type WebhookBitbucketPullRequestState string

const (
	BitbucketPRStateOpen     WebhookBitbucketPullRequestState = "OPEN"
	BitbucketPRStateMerged   WebhookBitbucketPullRequestState = "MERGED"
	BitbucketPRStateDeclined WebhookBitbucketPullRequestState = "DECLINED"
)

// WebhookBitbucketWorkspace defines the workspace object in Bitbucket Cloud.
type WebhookBitbucketWorkspace struct {
	Type  string                       `json:"type"`
	Slug  string                       `json:"slug"`
	Name  string                       `json:"name"`
	UUID  string                       `json:"uuid"`
	Links map[string]map[string]string `json:"links,omitempty"`
}

// WebhookBitbucketProject defines the project object in Bitbucket Cloud.
type WebhookBitbucketProject struct {
	Type  string                       `json:"type"`
	Name  string                       `json:"name"`
	UUID  string                       `json:"uuid"`
	Key   string                       `json:"key"`
	Links map[string]map[string]string `json:"links,omitempty"`
}

// WebhookBitbucketRepository defines the repository in Bitbucket Cloud.
type WebhookBitbucketRepository struct {
	Type      string                       `json:"type"`
	Name      string                       `json:"name"`
	FullName  string                       `json:"full_name"`
	Workspace WebhookBitbucketWorkspace    `json:"workspace"`
	UUID      string                       `json:"uuid"`
	Project   WebhookBitbucketProject      `json:"project"`
	Website   string                       `json:"website,omitempty"`
	SCM       string                       `json:"scm"`
	IsPrivate bool                         `json:"is_private"`
	Links     map[string]map[string]string `json:"links,omitempty"`
}

// WebhookBitbucketAccount defines user account metadata in Bitbucket Cloud.
type WebhookBitbucketAccount struct {
	DisplayName string `json:"display_name"`
	UUID        string `json:"uuid"`
	Type        string `json:"type"` // "user" | "team" | "app"
}

// WebhookBitbucketCommentInline holds position info for an inline comment.
type WebhookBitbucketCommentInline struct {
	To   *int   `json:"to,omitempty"`
	From *int   `json:"from,omitempty"`
	Path string `json:"path"`
}

// WebhookBitbucketComment defines a comment on a Bitbucket Cloud PR.
type WebhookBitbucketComment struct {
	ID      int `json:"id"`
	Parent  *struct {
		ID int `json:"id"`
	} `json:"parent,omitempty"`
	Content struct {
		Raw    string `json:"raw"`
		HTML   string `json:"html,omitempty"`
		Markup string `json:"markup,omitempty"`
	} `json:"content"`
	Inline    *WebhookBitbucketCommentInline `json:"inline,omitempty"`
	CreatedOn string                         `json:"created_on"`
	UpdatedOn string                         `json:"updated_on"`
	Links     map[string]map[string]string   `json:"links,omitempty"`
}

// WebhookBitbucketBranchRef holds branch and commit metadata for source/destination.
type WebhookBitbucketBranchRef struct {
	Branch struct {
		Name string `json:"name"`
	} `json:"branch"`
	Commit struct {
		Hash string `json:"hash"`
	} `json:"commit"`
	Repository WebhookBitbucketRepository `json:"repository"`
}

// WebhookBitbucketPullRequest defines a Bitbucket Cloud Pull Request.
type WebhookBitbucketPullRequest struct {
	ID                int                              `json:"id"`
	Title             string                           `json:"title"`
	Description       string                           `json:"description"`
	State             WebhookBitbucketPullRequestState `json:"state"`
	Author            WebhookBitbucketAccount          `json:"author"`
	Source            WebhookBitbucketBranchRef        `json:"source"`
	Destination       WebhookBitbucketBranchRef        `json:"destination"`
	MergeCommit       *struct{ Hash string `json:"hash"` } `json:"merge_commit,omitempty"`
	Participants      []WebhookBitbucketAccount        `json:"participants,omitempty"`
	Reviewers         []WebhookBitbucketAccount        `json:"reviewers,omitempty"`
	CloseSourceBranch bool                             `json:"close_source_branch"`
	ClosedBy          *WebhookBitbucketAccount         `json:"closed_by,omitempty"`
	Reason            string                           `json:"reason,omitempty"`
	CreatedOn         string                           `json:"created_on"`
	UpdatedOn         string                           `json:"updated_on"`
	Links             map[string]map[string]string     `json:"links,omitempty"`
	Draft             bool                             `json:"draft"`
}

// WebhookBitbucketPullRequestEvent is the payload for Bitbucket Cloud PR events.
type WebhookBitbucketPullRequestEvent struct {
	Actor             WebhookBitbucketAccount     `json:"actor"`
	PullRequest       WebhookBitbucketPullRequest `json:"pullrequest"`
	Repository        WebhookBitbucketRepository  `json:"repository"`
	Comment           *WebhookBitbucketComment    `json:"comment,omitempty"`
	IsDataCenterEvent bool                        `json:"isDataCenterEvent"` // false for Cloud
}

// --- Bitbucket Data Center / Server Types ---

// WebhookBitbucketDataCenterProject defines project info on Server/Data Center.
type WebhookBitbucketDataCenterProject struct {
	Key    string `json:"key"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Public bool   `json:"public"`
	Type   string `json:"type"`
}

// WebhookBitbucketDataCenterRepository defines repository info on Server/Data Center.
type WebhookBitbucketDataCenterRepository struct {
	Slug          string                            `json:"slug"`
	ID            int                               `json:"id"`
	Name          string                            `json:"name"`
	SCMID         string                            `json:"scmId"`
	State         string                            `json:"state"`
	StatusMessage string                            `json:"statusMessage,omitempty"`
	Forkable      bool                              `json:"forkable"`
	Project       WebhookBitbucketDataCenterProject `json:"project"`
}

// WebhookBitbucketDataCenterActor defines user info on Server/Data Center.
type WebhookBitbucketDataCenterActor struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
	ID           int    `json:"id"`
	DisplayName  string `json:"displayName"`
	Active       bool   `json:"active"`
	Slug         string `json:"slug"`
	Type         string `json:"type"`
}

// WebhookBitbucketDataCenterRef defines git ref info on Server/Data Center.
type WebhookBitbucketDataCenterRef struct {
	ID           string                               `json:"id"`
	DisplayID    string                               `json:"displayId"`
	LatestCommit string                               `json:"latestCommit"`
	Repository   WebhookBitbucketDataCenterRepository `json:"repository"`
}

// WebhookBitbucketDataCenterParticipant defines a reviewer or participant on Server.
type WebhookBitbucketDataCenterParticipant struct {
	User     WebhookBitbucketDataCenterActor `json:"user"`
	Role     string                          `json:"role"`
	Approved bool                            `json:"approved"`
	Status   string                          `json:"status"`
}

// WebhookBitbucketDataCenterPullRequest defines a PR on Bitbucket Server/Data Center.
type WebhookBitbucketDataCenterPullRequest struct {
	ID           int                                     `json:"id"`
	Version      int                                     `json:"version"`
	Title        string                                  `json:"title"`
	Description  string                                  `json:"description"`
	State        WebhookBitbucketPullRequestState        `json:"state"`
	Open         bool                                    `json:"open"`
	Closed       bool                                    `json:"closed"`
	Draft        bool                                    `json:"draft"`
	CreatedDate  string                                  `json:"createdDate"`
	UpdatedDate  string                                  `json:"updatedDate"`
	FromRef      WebhookBitbucketDataCenterRef           `json:"fromRef"`
	ToRef        WebhookBitbucketDataCenterRef           `json:"toRef"`
	Locked       bool                                    `json:"locked"`
	Author       WebhookBitbucketDataCenterParticipant   `json:"author"`
	Reviewers    []WebhookBitbucketDataCenterParticipant `json:"reviewers,omitempty"`
	Participants []WebhookBitbucketDataCenterParticipant `json:"participants,omitempty"`
}

// WebhookBitbucketDataCenterComment defines a comment on Bitbucket Server.
type WebhookBitbucketDataCenterComment struct {
	Properties struct {
		RepositoryID int `json:"repositoryId"`
	} `json:"properties,omitempty"`
	ID          int                                  `json:"id"`
	Version     int                                  `json:"version"`
	Text        string                               `json:"text"`
	Author      WebhookBitbucketDataCenterActor      `json:"author"`
	CreatedDate string                               `json:"createdDate"`
	UpdatedDate string                               `json:"updatedDate"`
	Comments    []*WebhookBitbucketDataCenterComment `json:"comments,omitempty"`
}

// WebhookBitbucketDataCenterPullRequestEvent is the payload for Server/DC events.
type WebhookBitbucketDataCenterPullRequestEvent struct {
	EventKey          string                                `json:"eventKey"`
	Date              string                                `json:"date"`
	Actor             WebhookBitbucketDataCenterActor       `json:"actor"`
	PullRequest       WebhookBitbucketDataCenterPullRequest `json:"pullrequest"`
	Comment           *WebhookBitbucketDataCenterComment    `json:"comment,omitempty"`
	CommentParentID   *int                                  `json:"commentParentId,omitempty"`
	IsDataCenterEvent bool                                  `json:"isDataCenterEvent"` // true for Server/DC
}

// StripCurlyBracesFromUUID cleans enclosing braces from Bitbucket UUID strings.
func StripCurlyBracesFromUUID(uuid string) string {
	uuid = strings.TrimSpace(uuid)
	if strings.HasPrefix(uuid, "{") && strings.HasSuffix(uuid, "}") {
		return uuid[1 : len(uuid)-1]
	}
	return uuid
}
